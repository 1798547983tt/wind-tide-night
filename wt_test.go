package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wind-tide: `new-api wt emergency-login` prints the owner's link on the site
// address the operator passes, never on the server address stored in the
// database, which an administrator could point at another host
// (wind-tide/API.md §4).
func TestWtEmergencyLoginCommand(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousSQLite, previousMaster := common.SQLitePath, common.IsMasterNode
	previousSecret, previousCrypto, previousAddress := common.SessionSecret, common.CryptoSecret, system_setting.ServerAddress
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.SQLitePath, common.IsMasterNode = previousSQLite, previousMaster
		common.SessionSecret, common.CryptoSecret, system_setting.ServerAddress = previousSecret, previousCrypto, previousAddress
	})
	t.Setenv("WT_OWNER_DISCORD_ID", "100000000000000001")
	t.Setenv("SESSION_SECRET", "wind-tide-test-secret")
	t.Setenv("SQL_DSN", "")
	t.Setenv("LOG_SQL_DSN", "")
	t.Setenv("WT_SITE_URL", "")
	t.Setenv("SQLITE_PATH", t.TempDir()+"/wt.db?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate")
	common.SQLitePath, common.IsMasterNode = os.Getenv("SQLITE_PATH"), true
	require.NoError(t, model.InitDB())
	require.NoError(t, model.InitLogDB())
	require.NoError(t, model.WtBootstrapOwner())
	const storedAddress = "https://attacker.example"
	require.NoError(t, model.DB.Create(&model.Option{Key: "ServerAddress", Value: storedAddress}).Error)
	system_setting.ServerAddress = storedAddress
	require.NoError(t, model.CloseDB())

	var tokens []string
	for _, issue := range []struct {
		name, siteURL string
		args          []string
		link          string
	}{
		{"no site address", "", []string{"emergency-login"}, "/emergency-login#token="},
		{"WT_SITE_URL", "https://wind.example/", []string{"emergency-login"}, "https://wind.example/emergency-login#token="},
		{"--base-url", "https://wind.example", []string{"emergency-login", "--base-url", "https://tide.example"}, "https://tide.example/emergency-login#token="},
	} {
		t.Setenv("WT_SITE_URL", issue.siteURL)
		var stdout, stderr bytes.Buffer
		require.Zero(t, runWtCommand(issue.args, &stdout, &stderr), stderr.String())
		token, found := strings.CutPrefix(strings.TrimSpace(stdout.String()), issue.link)
		require.True(t, found, issue.name+": "+stdout.String())
		require.NotEmpty(t, token, issue.name)
		assert.NotContains(t, stdout.String()+stderr.String(), "attacker.example", issue.name)
		assert.Equal(t, issue.siteURL == "", strings.Contains(stderr.String(), "No site address was given"), issue.name)
		tokens = append(tokens, token)
	}
	for _, args := range [][]string{
		{"emergency-login", "--base-url", "javascript:alert(1)"},
		{"emergency-login", "--base-url", "https://user@tide.example"},
		{"emergency-login", "--base-url", "https://tide.example/?next=elsewhere"},
		{"emergency-login", "--base-url", "tide.example"},
		{"emergency-login", "now"},
		{"reset-password"},
		{},
	} {
		var stdout, stderr bytes.Buffer
		assert.Equal(t, 2, runWtCommand(args, &stdout, &stderr), args)
		assert.Empty(t, stdout.String(), args)
	}

	// Each issued link is audited with its expiry, never with the token, and
	// only the newest link works.
	require.NoError(t, model.WtOpenDatabase())
	t.Cleanup(func() { _ = model.CloseDB() })
	var issued []model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ?", "wt.emergency_link_issued").Find(&issued).Error)
	require.Len(t, issued, len(tokens))
	var audits []model.AuditLog
	require.NoError(t, model.LOG_DB.Find(&audits).Error)
	for _, audit := range audits {
		encoded, err := common.Marshal(audit)
		require.NoError(t, err)
		if audit.Action == "wt.emergency_link_issued" {
			assert.Contains(t, string(encoded), `"expires_at"`)
		}
		for _, token := range tokens {
			digest := sha256.Sum256([]byte(token))
			assert.NotContains(t, string(encoded), token)
			assert.NotContains(t, string(encoded), hex.EncodeToString(digest[:]))
		}
	}
	assert.ErrorIs(t, model.ConsumeWtEmergencyToken(tokens[0], time.Now()), model.ErrWtEmergencyTokenInvalid)
	require.NoError(t, model.ConsumeWtEmergencyToken(tokens[len(tokens)-1], time.Now()))
}
