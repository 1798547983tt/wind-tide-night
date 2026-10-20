package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// wind-tide: regression tests of the fork's backend (wind-tide/API.md). Every
// test runs on SQLite, and also on MySQL and PostgreSQL when WT_TEST_MYSQL_DSN
// or WT_TEST_POSTGRES_DSN names a loopback server where the tests may create a
// throwaway database.

const (
	wtTestOwnerDiscordID = "100000000000000001"
	wtTestAvatarHash     = "0123456789abcdef0123456789abcdef"
)

var wtTestPNG = []byte("\x89PNG\r\n\x1a\nwind-tide")

// forEachWtDatabase runs test on a freshly migrated database of every
// configured dialect. The server startup migrations run twice to prove they
// are idempotent.
func forEachWtDatabase(t *testing.T, test func(t *testing.T)) {
	t.Helper()
	for _, dialect := range []struct{ kind, env string }{
		{"sqlite", ""},
		{"mysql", "WT_TEST_MYSQL_DSN"},
		{"postgres", "WT_TEST_POSTGRES_DSN"},
	} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip(dialect.env + " is not set")
			}
			setupWtDatabase(t, dialect.kind, dsn)
			test(t)
		})
	}
}

func setupWtDatabase(t *testing.T, kind, dsn string) {
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousMaster, previousSQLite := common.RedisEnabled, common.IsMasterNode, common.SQLitePath
	previousSecret, previousSetup, previousNewUserQuota := common.SessionSecret, constant.Setup, common.QuotaForNewUser
	previousCheckin, previousDiscord := *operation_setting.GetCheckinSetting(), *system_setting.GetDiscordSettings()
	common.RedisEnabled, common.IsMasterNode = false, true
	common.SessionSecret = "wind-tide-test-secret"
	common.QuotaForNewUser = 0
	*operation_setting.GetCheckinSetting() = operation_setting.CheckinSetting{
		Enabled:  true,
		MinQuota: int(9999 * common.QuotaPerUnit),
		MaxQuota: int(9999 * common.QuotaPerUnit),
	}
	// Discord sign-in is on, as wind-tide's defaults set it; it is also the
	// owner's step-up verification method.
	system_setting.GetDiscordSettings().Enabled = true
	t.Setenv("WT_OWNER_DISCORD_ID", wtTestOwnerDiscordID)
	t.Setenv("WT_AVATAR_DIR", t.TempDir())
	t.Setenv("LOG_SQL_DSN", "")
	if kind == "sqlite" {
		common.SQLitePath = t.TempDir() + "/wt.db?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate"
		t.Setenv("SQL_DSN", "")
	} else {
		_, isolated := newAuditTestDatabase(t, kind, dsn)
		t.Setenv("SQL_DSN", isolated)
	}
	t.Cleanup(func() {
		// The call counters are written off the request path; let them land
		// before the database goes.
		model.WtFlushCallCounters()
		if model.DB != previousDB {
			_ = model.CloseDB()
		}
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.RedisEnabled, common.IsMasterNode, common.SQLitePath = previousRedis, previousMaster, previousSQLite
		common.SessionSecret, constant.Setup, common.QuotaForNewUser = previousSecret, previousSetup, previousNewUserQuota
		*operation_setting.GetCheckinSetting(), *system_setting.GetDiscordSettings() = previousCheckin, previousDiscord
	})
	for startup := range 2 {
		require.NoError(t, model.InitDB())
		require.NoError(t, model.InitLogDB())
		if startup == 0 {
			require.NoError(t, model.CloseDB())
		}
	}
	wtLeaderboardMu.Lock()
	clear(wtLeaderboards)
	wtLeaderboardMu.Unlock()
	wtPublicStatsMu.Lock()
	wtPublicStatsCache = nil
	wtPublicStatsMu.Unlock()
}

// pinWtClock sets the clock behind check-in days and avatar addresses until
// the test ends.
func pinWtClock(t *testing.T, now time.Time) {
	t.Helper()
	previous := model.WtNow
	model.WtNow = func() time.Time { return now }
	t.Cleanup(func() { model.WtNow = previous })
}

// createWtUser stores a site account, by default an enabled member.
func createWtUser(t *testing.T, user model.User) *model.User {
	t.Helper()
	if user.Role == 0 {
		user.Role = common.RoleCommonUser
	}
	if user.Status == 0 {
		user.Status = common.UserStatusEnabled
	}
	user.AffCode = "wt-" + user.Username
	require.NoError(t, model.DB.Create(&user).Error)
	return &user
}

func admitWtAccount(t *testing.T, identity model.WtDiscordIdentity, now time.Time) *model.WtGateDecision {
	t.Helper()
	decision, err := model.WtAdmitDiscordAccount(identity, now)
	require.NoError(t, err)
	return decision
}

// cacheWtAvatar stores an avatar file the way the background download does.
func cacheWtAvatar(t *testing.T, discordID string) {
	t.Helper()
	path := service.WtAvatarPath(discordID, wtTestAvatarHash)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, wtTestPNG, 0o644))
}

// wtSession signs a user in with Discord and returns the browser session.
func wtSession(t *testing.T, userID int) service.AuthIdentity {
	t.Helper()
	bundle, err := service.CreateLoginSession(userID, "oauth:discord", "127.0.0.1", "wind-tide-test")
	require.NoError(t, err)
	identity, err := service.ParseAccessToken(bundle.AccessToken)
	require.NoError(t, err)
	return identity
}

// wtOwnerSession creates the owner's root account and signs the owner in.
func wtOwnerSession(t *testing.T) (*model.User, service.AuthIdentity) {
	t.Helper()
	require.NoError(t, model.WtBootstrapOwner())
	owner, err := model.WtOwnerAccount()
	require.NoError(t, err)
	return owner, wtSession(t, owner.Id)
}

// wtManageProof is the owner's step-up proof for disabling or enabling one
// account; with no password or second factor the owner verifies with Discord.
func wtManageProof(t *testing.T, owner service.AuthIdentity, userID int, action string) string {
	t.Helper()
	context, err := common.Marshal(service.AdminUserManageContext{UserID: userID, Action: action})
	require.NoError(t, err)
	return issueSecurityEnrollmentProof(t, owner, service.VerificationOperation{Scope: service.VerificationScopeAdminUserManage, Context: context}, service.VerificationMethodOAuth)
}

// wtRequest is one request to a handler, made the way the router makes it
// after authentication: identity is the signed-in browser session, if any.
type wtRequest struct {
	method, target, body string
	identity             service.AuthIdentity
	role                 int
	proof                string
	cookies              []*http.Cookie
	params               gin.Params
}

func (request wtRequest) serve(handler gin.HandlerFunc) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(request.method, request.target, strings.NewReader(request.body))
	c.Request.Header.Set("Content-Type", "application/json")
	if request.proof != "" {
		c.Request.Header.Set("X-Security-Proof", request.proof)
	}
	for _, cookie := range request.cookies {
		c.Request.AddCookie(cookie)
	}
	c.Params = request.params
	if request.identity.UserID != 0 {
		c.Set("id", request.identity.UserID)
		c.Set("session_id", request.identity.SessionID)
		c.Set("auth_version", request.identity.UserAuthVersion)
		c.Set("session_version", request.identity.SessionVersion)
	}
	if request.role != 0 {
		c.Set("role", request.role)
	}
	handler(c)
	c.Writer.WriteHeaderNow() // gin's engine sends a bare status after the handlers
	return recorder
}

// wtServe runs handler for one request of userID, 0 for anonymous requests.
func wtServe(handler gin.HandlerFunc, method, target, body string, userID int, params ...gin.Param) *httptest.ResponseRecorder {
	return wtRequest{method: method, target: target, body: body, identity: service.AuthIdentity{UserID: userID}, params: params}.serve(handler)
}

// wtDiscordSignIn finishes a Discord sign-in after the OAuth exchange.
func wtDiscordSignIn(user *oauth.OAuthUser) *httptest.ResponseRecorder {
	return wtServe(func(c *gin.Context) {
		c.Set("login_method", "oauth:discord")
		wtHandleDiscordLogin(c, user)
	}, http.MethodGet, "/api/oauth/discord?code=test&state=test", "", 0)
}

// wtAllowlistStatus removes or restores an allowlist entry over HTTP as the
// owner.
func wtAllowlistStatus(handler gin.HandlerFunc, owner service.AuthIdentity, proof string, entryID int) *httptest.ResponseRecorder {
	id := strconv.Itoa(entryID)
	return wtRequest{method: http.MethodPost, target: "/api/wt/allowlist/" + id, identity: owner, role: common.RoleRootUser, proof: proof,
		params: gin.Params{{Key: "id", Value: id}}}.serve(handler)
}

// wtListedEntry is the allowlist entry of a Discord ID as the owner sees it.
func wtListedEntry(t *testing.T, discordID string) model.WtAllowlistItem {
	t.Helper()
	items, _, err := model.WtListAllowlist(discordID, "", 0, 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	return items[0]
}

// addWtLegacyPrefill stores an unpinned username pre-fill the way the owner
// added one before the allowlist took Discord user IDs only.
func addWtLegacyPrefill(t *testing.T, username string) model.WtAllowlistEntry {
	t.Helper()
	key := strings.ToLower(username)
	now := time.Now().Unix()
	entry := model.WtAllowlistEntry{DiscordUsername: username, UsernameKey: &key, Status: model.WtAllowlistActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, model.DB.Create(&entry).Error)
	return entry
}

// wtEntryRequestStatus is the state of the entry request of a Discord ID, ""
// when there is none.
func wtEntryRequestStatus(t *testing.T, discordID string) string {
	t.Helper()
	var request model.WtEntryRequest
	require.NoError(t, model.DB.Where("discord_id = ?", discordID).Limit(1).Find(&request).Error)
	return request.Status
}

// wtAudits returns the audit entries of an action, oldest first.
func wtAudits(t *testing.T, action string) []model.AuditLog {
	t.Helper()
	var audits []model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ?", action).Order("id").Find(&audits).Error)
	return audits
}

// wtAuditParams returns the params of an audit entry as JSON.
func wtAuditParams(t *testing.T, audit model.AuditLog) string {
	t.Helper()
	require.NotNil(t, audit.Other.Op)
	params, err := common.Marshal(audit.Other.Op.Params)
	require.NoError(t, err)
	return string(params)
}

// createWtAPIKey stores an unlimited API key of the user and returns it.
func createWtAPIKey(t *testing.T, userID int, status int) string {
	t.Helper()
	key, err := common.GenerateKey()
	require.NoError(t, err)
	token := model.Token{UserId: userID, Key: key, Name: "relay", Status: status, ExpiredTime: -1, UnlimitedQuota: true, CreatedTime: time.Now().Unix()}
	require.NoError(t, token.Insert())
	return key
}

// newWtRelay is a relay route that answers 204 once the API key passed the
// relay authentication and the model rate limit.
func newWtRelay() *gin.Engine {
	relay := gin.New()
	relay.POST("/v1/chat/completions", middleware.TokenAuth(), middleware.ModelRequestRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	return relay
}

func wtRelayStatus(relay *gin.Engine, key string) int {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
	request.Header.Set("Authorization", "Bearer sk-"+key)
	relay.ServeHTTP(recorder, request)
	return recorder.Code
}

// wtDashboardStatus is the status of a dashboard request made with an access
// token, a personal one or the legacy one.
func wtDashboardStatus(dashboard *gin.Engine, credential string) int {
	return accessTokenRequest(dashboard, http.MethodGet, "/api/user/self", credential, "", "").Code
}

// wtDiscordAPI answers the OAuth calls of a Discord sign-in as the Discord
// account userID and counts them.
type wtDiscordAPI struct {
	userID string
	calls  atomic.Int32
}

func (api *wtDiscordAPI) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Body != nil {
		_ = request.Body.Close()
	}
	if request.URL.Host != "discord.com" {
		return nil, fmt.Errorf("unexpected request to %s", request.URL.Host)
	}
	api.calls.Add(1)
	body := `{"access_token":"wind-tide-test","token_type":"Bearer","scope":"identify"}`
	if request.URL.Path == "/api/v10/users/@me" {
		body = `{"id":"` + api.userID + `","username":"tide","global_name":"Tide Keeper","avatar":null}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}, nil
}

// stubWtDiscord sends the requests to Discord to a wtDiscordAPI until the test
// ends.
func stubWtDiscord(t *testing.T, userID string) *wtDiscordAPI {
	t.Helper()
	api := &wtDiscordAPI{userID: userID}
	previous := http.DefaultTransport
	http.DefaultTransport = api
	t.Cleanup(func() { http.DefaultTransport = previous })
	return api
}

// wtStartDiscord starts a Discord OAuth flow and returns its state and the
// cookies the browser got.
func wtStartDiscord(t *testing.T, request wtRequest) (string, []*http.Cookie) {
	t.Helper()
	request.method, request.target = http.MethodPost, "/api/oauth/state"
	recorder := request.serve(GenerateOAuthCode)
	response := decodeSecurityEnrollmentResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	var started struct {
		FlowToken string `json:"flow_token"`
	}
	require.NoError(t, common.Unmarshal(response.Data, &started))
	return started.FlowToken, recorder.Result().Cookies()
}

// wtFinishDiscord is the Discord OAuth callback of a flow.
func wtFinishDiscord(request wtRequest, state string) *httptest.ResponseRecorder {
	request.method, request.target = http.MethodGet, "/api/oauth/discord?code=test&state="+url.QueryEscape(state)
	request.params = gin.Params{{Key: "provider", Value: "discord"}}
	return request.serve(HandleOAuth)
}

func TestWtSignInGate(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
		pinWtClock(t, now)

		// The owner account is created once, without a password, and the site
		// counts as set up.
		require.NoError(t, model.WtBootstrapOwner())
		require.NoError(t, model.WtBootstrapOwner())
		var roots []model.User
		require.NoError(t, model.DB.Where("role = ?", common.RoleRootUser).Find(&roots).Error)
		require.Len(t, roots, 1)
		assert.Equal(t, wtTestOwnerDiscordID, roots[0].DiscordId)
		assert.Empty(t, roots[0].Password)
		constant.Setup = false
		model.CheckSetup()
		assert.True(t, constant.Setup)

		owner := admitWtAccount(t, model.WtDiscordIdentity{DiscordId: wtTestOwnerDiscordID, Username: "tide", GlobalName: "Tide Keeper"}, now)
		require.NotNil(t, owner.User)
		assert.Equal(t, roots[0].Id, owner.User.Id)
		assert.Equal(t, "Tide Keeper", owner.User.DisplayName)

		// An unknown account files one entry request; signing in again refreshes
		// the names and keeps the request time.
		stranger := model.WtDiscordIdentity{DiscordId: "200000000000000002", Username: "drifter", GlobalName: "Drifter"}
		first := admitWtAccount(t, stranger, now)
		assert.Equal(t, &model.WtGateDecision{Gate: model.WtEntryPending, RequestedAt: now.Unix()}, first)
		again := admitWtAccount(t, model.WtDiscordIdentity{DiscordId: stranger.DiscordId, Username: "drifter2", GlobalName: "Drifter Two"}, now.Add(time.Hour))
		assert.Equal(t, &model.WtGateDecision{Gate: model.WtEntryPending, RequestedAt: now.Unix()}, again)
		requests, total, err := model.WtListEntryRequests(model.WtEntryPending, 0, 10)
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		assert.Equal(t, "drifter2", requests[0].DiscordUsername)
		assert.Equal(t, "Drifter Two", requests[0].DisplayName)

		// A rejected applicant is told when to retry; from then on the next
		// sign-in files a fresh request.
		rejectedAt := now.Add(2 * time.Hour)
		_, err = model.WtRejectEntryRequest(requests[0].Id, rejectedAt)
		require.NoError(t, err)
		_, err = model.WtRejectEntryRequest(requests[0].Id, rejectedAt)
		assert.ErrorIs(t, err, model.ErrWtEntryRequestDecided)
		retryAt := rejectedAt.Add(model.WtEntryRetryDelay)
		blocked := admitWtAccount(t, stranger, retryAt.Add(-time.Second))
		assert.Equal(t, &model.WtGateDecision{Gate: model.WtEntryRejected, RequestedAt: now.Unix(), RetryAfter: retryAt.Unix()}, blocked)
		reopened := admitWtAccount(t, stranger, retryAt)
		assert.Equal(t, &model.WtGateDecision{Gate: model.WtEntryPending, RequestedAt: retryAt.Unix()}, reopened)

		// Approval admits the applicant as a member with the new-user quota.
		approved, err := model.WtApproveEntryRequest(requests[0].Id, retryAt.Add(time.Minute))
		require.NoError(t, err)
		assert.Equal(t, model.WtEntryApproved, approved.Status)
		_, err = model.WtApproveEntryRequest(requests[0].Id, retryAt.Add(time.Minute))
		assert.ErrorIs(t, err, model.ErrWtEntryRequestDecided)
		member := admitWtAccount(t, stranger, retryAt.Add(2*time.Minute))
		require.NotNil(t, member.User)
		assert.Equal(t, common.RoleCommonUser, member.User.Role)
		assert.Equal(t, "drifter", member.User.Username)
		assert.Equal(t, "Drifter", member.User.DisplayName)
		assert.Equal(t, stranger.DiscordId, member.User.DiscordId)
		assert.Zero(t, member.User.Quota)
		returning := admitWtAccount(t, stranger, retryAt.Add(time.Hour))
		require.NotNil(t, returning.User)
		assert.Equal(t, member.User.Id, returning.User.Id)

		// An entry added by Discord ID admits that account. A taken username
		// falls back to discord_<n>, skipping generated names that are taken.
		blocker := createWtUser(t, model.User{Username: "blocker"})
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", blocker.Id).Update("username", "discord_"+strconv.Itoa(blocker.Id+1)).Error)
		_, err = model.WtAddAllowlistEntry(model.WtAllowlistInput{DiscordId: "500000000000000005", Note: "direct"}, now)
		require.NoError(t, err)
		listed := admitWtAccount(t, model.WtDiscordIdentity{DiscordId: "500000000000000005", Username: "drifter"}, now)
		require.NotNil(t, listed.User)
		assert.Equal(t, "discord_"+strconv.Itoa(blocker.Id+2), listed.User.Username)

		// A username pre-fill added before the allowlist took IDs only is
		// pinned to the first Discord ID that signs in with it; another
		// account using the same name later is not admitted.
		prefill := addWtLegacyPrefill(t, "PreFill")
		pinned := admitWtAccount(t, model.WtDiscordIdentity{DiscordId: "300000000000000003", Username: "prefill"}, now)
		require.NotNil(t, pinned.User)
		impostor := admitWtAccount(t, model.WtDiscordIdentity{DiscordId: "400000000000000004", Username: "PreFill"}, now)
		assert.Equal(t, &model.WtGateDecision{Gate: model.WtEntryPending, RequestedAt: now.Unix()}, impostor)
		entries, _, err := model.WtListAllowlist("prefill", "", 0, 10)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, prefill.Id, entries[0].Id)
		assert.Equal(t, "300000000000000003", entries[0].DiscordId)
		assert.Equal(t, pinned.User.Id, entries[0].UserId)

		// Adding an applicant's ID approves the pending request.
		_, err = model.WtAddAllowlistEntry(model.WtAllowlistInput{DiscordId: "400000000000000004"}, now)
		require.NoError(t, err)
		pending, err := model.WtCountPendingEntryRequests()
		require.NoError(t, err)
		assert.Zero(t, pending)

		// A removed account is turned away without a new entry request.
		accountID, err := model.WtAllowlistStatusAccount(prefill.Id, model.WtAllowlistRemoved)
		require.NoError(t, err)
		assert.Equal(t, pinned.User.Id, accountID)
		_, _, err = model.WtRemoveAllowlistEntry(prefill.Id, accountID, now)
		require.NoError(t, err)
		removed := admitWtAccount(t, model.WtDiscordIdentity{DiscordId: "300000000000000003", Username: "prefill"}, now)
		assert.Equal(t, &model.WtGateDecision{Gate: model.WtGateRemoved}, removed)
		assert.Empty(t, wtEntryRequestStatus(t, "300000000000000003"))

		// Over HTTP an applicant gets the gate state and no session.
		applicantID := "600000000000000006"
		cacheWtAvatar(t, applicantID)
		gated := wtDiscordSignIn(&oauth.OAuthUser{ProviderUserID: applicantID, Username: "newcomer", DisplayName: "New Comer", Extra: map[string]any{"avatar": wtTestAvatarHash}})
		assert.Equal(t, http.StatusOK, gated.Code)
		assert.Empty(t, gated.Result().Cookies())
		assert.Equal(t, "no-store", gated.Header().Get("Cache-Control"))
		gatedBody := decodeSecurityEnrollmentResponse(t, gated)
		assert.False(t, gatedBody.Success)
		assert.Equal(t, "Your entry request is waiting for approval", gatedBody.Message)
		var gate struct {
			Gate      string `json:"wt_gate"`
			Applicant struct {
				DiscordId   string `json:"discord_id"`
				Username    string `json:"username"`
				DisplayName string `json:"display_name"`
				AvatarURL   string `json:"avatar_url"`
			} `json:"applicant"`
			RequestedAt int64 `json:"requested_at"`
			RetryAfter  int64 `json:"retry_after"`
		}
		require.NoError(t, common.Unmarshal(gatedBody.Data, &gate))
		assert.Equal(t, model.WtEntryPending, gate.Gate)
		assert.Equal(t, applicantID, gate.Applicant.DiscordId)
		assert.Equal(t, "newcomer", gate.Applicant.Username)
		assert.Equal(t, "New Comer", gate.Applicant.DisplayName)
		assert.NotEmpty(t, gate.Applicant.AvatarURL)
		assert.Equal(t, model.WtAvatarURL(applicantID, wtTestAvatarHash), gate.Applicant.AvatarURL)
		assert.Positive(t, gate.RequestedAt)
		assert.Zero(t, gate.RetryAfter)

		// The owner signs in with a session and sees the cached avatar.
		cacheWtAvatar(t, wtTestOwnerDiscordID)
		signedIn := wtDiscordSignIn(&oauth.OAuthUser{ProviderUserID: wtTestOwnerDiscordID, Username: "tide", DisplayName: "Tide Keeper", Extra: map[string]any{"avatar": wtTestAvatarHash}})
		signedInBody := decodeSecurityEnrollmentResponse(t, signedIn)
		require.True(t, signedInBody.Success, signedInBody.Message)
		var login struct {
			AccessToken string `json:"access_token"`
			User        struct {
				Id        int    `json:"id"`
				AvatarURL string `json:"avatar_url"`
			} `json:"user"`
		}
		require.NoError(t, common.Unmarshal(signedInBody.Data, &login))
		assert.NotEmpty(t, login.AccessToken)
		assert.Equal(t, roots[0].Id, login.User.Id)
		assert.Equal(t, model.WtAvatarURL(wtTestOwnerDiscordID, wtTestAvatarHash), login.User.AvatarURL)
		assert.NotEmpty(t, signedIn.Result().Cookies())
	})
}

// The allowlist adds Discord user IDs only: anyone can take a Discord username
// nobody holds, so a name identifies nobody. A username pre-fill added before
// that admits one Discord account only: the applicant the owner approves, or
// else the first account that signs in with the name. Pinning it approves that
// account's entry request.
func TestWtUsernamePrefill(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		now := time.Now()
		_, owner := wtOwnerSession(t)

		// A name without an ID is refused, also while an applicant waits under
		// it; the applicant keeps waiting for the owner's answer.
		alice := model.WtDiscordIdentity{DiscordId: "810000000000000081", Username: "alice"}
		assert.Equal(t, model.WtEntryPending, admitWtAccount(t, alice, now).Gate)
		for _, body := range []string{`{"discord_username":"@Alice"}`, `{"discord_id":" ","discord_username":"alice","note":"friend"}`} {
			added := decodeSecurityEnrollmentResponse(t, wtRequest{method: http.MethodPost, target: "/api/wt/allowlist", body: body,
				identity: owner, role: common.RoleRootUser}.serve(WtAddAllowlistEntry))
			assert.False(t, added.Success, body)
			assert.Equal(t, "Enter a Discord user ID", added.Message, body)
		}
		_, listed, err := model.WtListAllowlist("", "", 0, 10)
		require.NoError(t, err)
		assert.Zero(t, listed)
		assert.Equal(t, model.WtEntryPending, wtEntryRequestStatus(t, alice.DiscordId))

		// With applicants waiting under a pre-fill's name it stays open until
		// the owner approves one of them; it is then pinned to that one.
		carol := model.WtDiscordIdentity{DiscordId: "830000000000000083", Username: "carol"}
		namesake := model.WtDiscordIdentity{DiscordId: "840000000000000084", Username: "Carol"}
		admitWtAccount(t, carol, now)
		admitWtAccount(t, namesake, now)
		open := addWtLegacyPrefill(t, "carol")
		var carolRequest model.WtEntryRequest
		require.NoError(t, model.DB.Where("discord_id = ?", carol.DiscordId).First(&carolRequest).Error)
		_, err = model.WtApproveEntryRequest(carolRequest.Id, now)
		require.NoError(t, err)
		entries, _, err := model.WtListAllowlist("carol", "", 0, 10)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, open.Id, entries[0].Id)
		assert.Equal(t, carol.DiscordId, entries[0].DiscordId)
		assert.Equal(t, model.WtEntryPending, admitWtAccount(t, namesake, now).Gate)
		require.NotNil(t, admitWtAccount(t, carol, now).User)

		// A pre-fill is pinned at the first sign-in with the name, which
		// approves the account's earlier request and is audited once.
		wanderer := model.WtDiscordIdentity{DiscordId: "850000000000000085", Username: "wanderer_old"}
		admitWtAccount(t, wanderer, now)
		prefill := addWtLegacyPrefill(t, "Wanderer")
		for range 2 {
			signedIn := decodeSecurityEnrollmentResponse(t, wtDiscordSignIn(&oauth.OAuthUser{ProviderUserID: wanderer.DiscordId, Username: "wanderer"}))
			require.True(t, signedIn.Success, signedIn.Message)
		}
		assert.Equal(t, model.WtEntryApproved, wtEntryRequestStatus(t, wanderer.DiscordId))
		pinned := wtListedEntry(t, wanderer.DiscordId)
		assert.Equal(t, prefill.Id, pinned.Id)
		pins := wtAudits(t, "wt.allowlist_pin")
		require.Len(t, pins, 1)
		assert.Equal(t, pinned.UserId, pins[0].UserId)
		assert.JSONEq(t, fmt.Sprintf(`{"id":%d,"discord_id":"850000000000000085","discord_username":"wanderer"}`, prefill.Id), wtAuditParams(t, pins[0]))
	})
}

// The owner can approve an applicant while the applicant signs in again; that
// sign-in then admits the new member instead of reopening the request. Only
// PostgreSQL lets the approval commit in the middle of the sign-in: SQLite
// runs one writer at a time and MySQL's gap locks hold the approval back.
func TestWtSignInWhileTheOwnerApproves(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		if !common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
			t.Skip("only PostgreSQL lets the approval commit in the middle of the sign-in")
		}
		now := time.Now()
		applicant := model.WtDiscordIdentity{DiscordId: "860000000000000086", Username: "racer"}
		require.Equal(t, model.WtEntryPending, admitWtAccount(t, applicant, now).Gate)
		var request model.WtEntryRequest
		require.NoError(t, model.DB.Where("discord_id = ?", applicant.DiscordId).First(&request).Error)

		// The approval commits right before the sign-in files the request again.
		approved := false
		require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register("wt_test:approve", func(tx *gorm.DB) {
			if approved || tx.Statement.Table != (model.WtEntryRequest{}).TableName() {
				return
			}
			approved = true
			_, err := model.WtApproveEntryRequest(request.Id, now)
			assert.NoError(t, err)
		}))
		t.Cleanup(func() { _ = model.DB.Callback().Create().Remove("wt_test:approve") })

		decision := admitWtAccount(t, applicant, now)
		assert.True(t, approved)
		require.NotNil(t, decision.User)
		assert.Equal(t, applicant.DiscordId, decision.User.DiscordId)
		assert.Equal(t, model.WtEntryApproved, wtEntryRequestStatus(t, applicant.DiscordId))
	})
}

// Names are stored best-effort: a MySQL database in utf8mb3 cannot store the
// emoji Discord allows in names, and that must keep nobody out.
func TestWtNamesAreStoredBestEffort(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		utf8mb3 := common.UsingMainDatabase(common.DatabaseTypeMySQL)
		if utf8mb3 {
			for _, table := range []string{"users", "wt_discord_profiles", "wt_entry_requests"} {
				require.NoError(t, model.DB.Exec("ALTER TABLE "+table+" CONVERT TO CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci").Error)
			}
		}
		now := time.Now()
		require.NoError(t, model.WtBootstrapOwner())
		owner := admitWtAccount(t, model.WtDiscordIdentity{DiscordId: wtTestOwnerDiscordID, Username: "tide", GlobalName: "Tide 🌊"}, now)
		require.NotNil(t, owner.User)
		_, err := model.WtAddAllowlistEntry(model.WtAllowlistInput{DiscordId: "870000000000000087"}, now)
		require.NoError(t, err)
		member := admitWtAccount(t, model.WtDiscordIdentity{DiscordId: "870000000000000087", Username: "surfer", GlobalName: "Surfer 🏄"}, now)
		require.NotNil(t, member.User)
		applicant := admitWtAccount(t, model.WtDiscordIdentity{DiscordId: "880000000000000088", Username: "sailor", GlobalName: "Sailor 🚢"}, now)
		assert.Equal(t, model.WtEntryPending, applicant.Gate)
		requests, _, err := model.WtListEntryRequests(model.WtEntryPending, 0, 10)
		require.NoError(t, err)
		require.Len(t, requests, 1)
		names := []string{owner.User.DisplayName, member.User.DisplayName, requests[0].DisplayName}
		if utf8mb3 {
			assert.Equal(t, []string{"Root User", "surfer", "sailor"}, names)
		} else {
			assert.Equal(t, []string{"Tide 🌊", "Surfer 🏄", "Sailor 🚢"}, names)
		}
	})
}

// Avatars are served only at a signed address that has not expired, and only
// for Discord IDs of accounts and entry requests.
func TestWtAvatarAddresses(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)       // 20:00 in Beijing
		expiresAt := time.Date(2026, 10, 15, 16, 0, 0, 0, time.UTC) // the end of the next Beijing day
		pinWtClock(t, now)
		member := createWtUser(t, model.User{Username: "member", DiscordId: "890000000000000089"})
		applicantID, strangerID := "900000000000000090", "910000000000000091"
		require.NoError(t, model.DB.Create(&model.WtEntryRequest{DiscordId: applicantID, DiscordUsername: "applicant", DisplayName: "applicant", Status: model.WtEntryPending, RequestedAt: now.Unix()}).Error)
		for _, discordID := range []string{member.DiscordId, applicantID, strangerID} {
			require.NoError(t, model.DB.Create(&model.WtDiscordProfile{DiscordId: discordID, Username: "someone", AvatarHash: wtTestAvatarHash, UpdatedAt: now.Unix()}).Error)
			cacheWtAvatar(t, discordID)
		}
		address := func(discordID string) url.Values {
			parsed, err := url.Parse(model.WtAvatarURL(discordID, wtTestAvatarHash))
			require.NoError(t, err)
			require.Equal(t, "/api/wt/avatar/"+discordID, parsed.Path)
			return parsed.Query()
		}
		fetch := func(discordID string, query url.Values) *httptest.ResponseRecorder {
			return wtRequest{method: http.MethodGet, target: "/api/wt/avatar/" + discordID + "?" + query.Encode(), params: gin.Params{{Key: "discord_id", Value: discordID}}}.serve(WtGetAvatar)
		}

		signed := address(member.DiscordId)
		assert.Equal(t, strconv.FormatInt(expiresAt.Unix(), 10), signed.Get("exp"))
		served := fetch(member.DiscordId, signed)
		assert.Equal(t, http.StatusOK, served.Code)
		assert.Equal(t, "public, max-age=86400", served.Header().Get("Cache-Control"))
		assert.Equal(t, wtTestPNG, served.Body.Bytes())
		assert.Equal(t, http.StatusOK, fetch(applicantID, address(applicantID)).Code)

		withoutSignature, withoutExpiry := maps.Clone(signed), maps.Clone(signed)
		withoutSignature.Del("sig")
		withoutExpiry.Del("exp")
		forged, extended := maps.Clone(signed), maps.Clone(signed)
		forged.Set("sig", strings.Repeat("0", len(signed.Get("sig"))))
		extended.Set("exp", strconv.FormatInt(expiresAt.Add(24*time.Hour).Unix(), 10))
		for name, request := range map[string]struct {
			discordID string
			query     url.Values
		}{
			"unsigned":                  {member.DiscordId, url.Values{"v": {wtTestAvatarHash}}},
			"no signature":              {member.DiscordId, withoutSignature},
			"no expiry":                 {member.DiscordId, withoutExpiry},
			"forged signature":          {member.DiscordId, forged},
			"extended expiry":           {member.DiscordId, extended},
			"another account's address": {applicantID, signed},
			"unknown Discord ID":        {strangerID, address(strangerID)},
		} {
			assert.Equal(t, http.StatusNotFound, fetch(request.discordID, request.query).Code, name)
		}

		// A browser keeps an avatar no longer than its address is valid, and
		// the address stops working when it expires.
		pinWtClock(t, expiresAt.Add(-time.Hour))
		lastHour := fetch(member.DiscordId, signed)
		assert.Equal(t, http.StatusOK, lastHour.Code)
		assert.Equal(t, "public, max-age=3600", lastHour.Header().Get("Cache-Control"))
		pinWtClock(t, expiresAt)
		assert.Equal(t, http.StatusNotFound, fetch(member.DiscordId, signed).Code)
	})
}

func TestWtAllowlistRemovalRevokesAccess(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		now := time.Now()
		_, owner := wtOwnerSession(t)
		identity := model.WtDiscordIdentity{DiscordId: "700000000000000007", Username: "member"}
		entry, err := model.WtAddAllowlistEntry(model.WtAllowlistInput{DiscordId: identity.DiscordId}, now)
		require.NoError(t, err)
		member := admitWtAccount(t, identity, now).User
		require.NotNil(t, member)
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", member.Id).Update("quota", 12345).Error)

		session := wtSession(t, member.Id)
		key := createWtAPIKey(t, member.Id, common.TokenStatusEnabled)
		pat, _ := createScopedAccessToken(t, member.Id, 0, "profile:read")
		relay, dashboard := newWtRelay(), newAccessTokenTestRouter()
		require.Equal(t, http.StatusNoContent, wtRelayStatus(relay, key))
		require.Equal(t, http.StatusOK, wtDashboardStatus(dashboard, pat))

		// Removal ends the browser session, deletes the access token and
		// switches the API key off.
		removed, revoked, err := model.WtRemoveAllowlistEntry(entry.Id, member.Id, now)
		require.NoError(t, err)
		assert.Equal(t, model.WtRevokedCredentials{AccessTokens: 1, RelayKeys: 1}, revoked)
		assert.Equal(t, model.WtAllowlistRemoved, removed.Status)
		assert.Equal(t, member.Id, removed.UserId)
		assert.Equal(t, model.WtAccountDisabled, removed.AccountStatus)
		_, _, err = service.ValidateLoginSession(session)
		assert.Error(t, err)
		assert.Equal(t, http.StatusUnauthorized, wtRelayStatus(relay, key))
		assert.Equal(t, http.StatusUnauthorized, wtDashboardStatus(dashboard, pat))
		assert.Equal(t, model.WtGateRemoved, admitWtAccount(t, identity, now).Gate)

		// Only restoring the entry enables the account again; user management
		// is refused even with the owner's verification.
		account, err := model.GetUserById(member.Id, false)
		require.NoError(t, err)
		account.Status = common.UserStatusEnabled
		assert.ErrorIs(t, account.Update(false), model.ErrWtApplicantRemoved)
		enabled := decodeSecurityEnrollmentResponse(t, wtRequest{method: http.MethodPost, target: "/api/user/manage", body: fmt.Sprintf(`{"id":%d,"action":"enable"}`, member.Id),
			identity: owner, role: common.RoleRootUser, proof: wtManageProof(t, owner, member.Id, "enable")}.serve(ManageUser))
		assert.False(t, enabled.Success)
		assert.Equal(t, model.ErrWtApplicantRemoved.Error(), enabled.Message)
		account, err = model.GetUserById(member.Id, false)
		require.NoError(t, err)
		assert.Equal(t, common.UserStatusDisabled, account.Status)

		// Restoring enables the account with its balance. The session and the
		// access token stay revoked and the API key stays off until the member
		// switches it on again.
		restored, err := model.WtRestoreAllowlistEntry(entry.Id, member.Id, now)
		require.NoError(t, err)
		assert.Equal(t, model.WtAllowlistActive, restored.Status)
		assert.Equal(t, model.WtAccountEnabled, restored.AccountStatus)
		account, err = model.GetUserById(member.Id, false)
		require.NoError(t, err)
		assert.Equal(t, common.UserStatusEnabled, account.Status)
		assert.Equal(t, 12345, account.Quota)
		_, _, err = service.ValidateLoginSession(session)
		assert.Error(t, err)
		assert.Equal(t, http.StatusUnauthorized, wtRelayStatus(relay, key))
		assert.Equal(t, http.StatusUnauthorized, wtDashboardStatus(dashboard, pat))
		returning := admitWtAccount(t, identity, now)
		require.NotNil(t, returning.User)
		assert.Equal(t, member.Id, returning.User.Id)
		require.NoError(t, model.DB.Model(&model.Token{}).Where("user_id = ?", member.Id).Update("status", common.TokenStatusEnabled).Error)
		assert.Equal(t, http.StatusNoContent, wtRelayStatus(relay, key))
	})
}

// Removing or restoring an entry that has an account disables or enables that
// account, so it takes the owner's step-up verification for exactly that
// account and action, like upstream user management.
func TestWtAllowlistStatusTakesStepUp(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		now := time.Now()
		_, owner := wtOwnerSession(t)
		keeper := model.WtDiscordIdentity{DiscordId: "710000000000000071", Username: "keeper"}
		entry, err := model.WtAddAllowlistEntry(model.WtAllowlistInput{DiscordId: keeper.DiscordId}, now)
		require.NoError(t, err)
		member := admitWtAccount(t, keeper, now).User
		require.NotNil(t, member)
		bystander := createWtUser(t, model.User{Username: "bystander"})

		for _, attempt := range []struct{ name, proof, code string }{
			{"no proof", "", "SECURITY_PROOF_REQUIRED"},
			{"proof for another account", wtManageProof(t, owner, bystander.Id, "disable"), "SECURITY_PROOF_CONTEXT_MISMATCH"},
			{"proof for enabling", wtManageProof(t, owner, member.Id, "enable"), "SECURITY_PROOF_CONTEXT_MISMATCH"},
		} {
			refused := wtAllowlistStatus(WtRemoveAllowlistEntry, owner, attempt.proof, entry.Id)
			assert.Equal(t, http.StatusForbidden, refused.Code, attempt.name)
			assert.Equal(t, attempt.code, decodeSecurityEnrollmentResponse(t, refused).Code, attempt.name)
		}
		listed := wtListedEntry(t, keeper.DiscordId)
		assert.Equal(t, model.WtAllowlistActive, listed.Status)
		assert.Equal(t, model.WtAccountEnabled, listed.AccountStatus)

		removed := decodeSecurityEnrollmentResponse(t, wtAllowlistStatus(WtRemoveAllowlistEntry, owner, wtManageProof(t, owner, member.Id, "disable"), entry.Id))
		require.True(t, removed.Success, removed.Message)
		removals := wtAudits(t, "wt.allowlist_remove")
		require.Len(t, removals, 1)
		assert.JSONEq(t, fmt.Sprintf(`{"id":%d,"discord_id":"710000000000000071","discord_username":"keeper","status":"removed","verification_method":"oauth","access_tokens":0,"relay_keys":0,"target_user_id":%d}`, entry.Id, member.Id), wtAuditParams(t, removals[0]))

		for _, attempt := range []struct{ name, proof, code string }{
			{"no proof", "", "SECURITY_PROOF_REQUIRED"},
			{"proof for disabling", wtManageProof(t, owner, member.Id, "disable"), "SECURITY_PROOF_CONTEXT_MISMATCH"},
		} {
			refused := wtAllowlistStatus(WtRestoreAllowlistEntry, owner, attempt.proof, entry.Id)
			assert.Equal(t, http.StatusForbidden, refused.Code, attempt.name)
			assert.Equal(t, attempt.code, decodeSecurityEnrollmentResponse(t, refused).Code, attempt.name)
		}
		restored := decodeSecurityEnrollmentResponse(t, wtAllowlistStatus(WtRestoreAllowlistEntry, owner, wtManageProof(t, owner, member.Id, "enable"), entry.Id))
		require.True(t, restored.Success, restored.Message)
		restores := wtAudits(t, "wt.allowlist_restore")
		require.Len(t, restores, 1)
		assert.JSONEq(t, fmt.Sprintf(`{"id":%d,"discord_id":"710000000000000071","discord_username":"keeper","status":"active","verification_method":"oauth","target_user_id":%d}`, entry.Id, member.Id), wtAuditParams(t, restores[0]))
		_, err = model.WtAllowlistStatusAccount(entry.Id, model.WtAllowlistActive)
		assert.ErrorIs(t, err, model.ErrWtAllowlistNotRemoved)

		// An entry without an account changes no account and takes no proof.
		idOnly, err := model.WtAddAllowlistEntry(model.WtAllowlistInput{DiscordId: "720000000000000072"}, now)
		require.NoError(t, err)
		for _, handler := range []gin.HandlerFunc{WtRemoveAllowlistEntry, WtRestoreAllowlistEntry} {
			changed := decodeSecurityEnrollmentResponse(t, wtAllowlistStatus(handler, owner, "", idOnly.Id))
			assert.True(t, changed.Success, changed.Message)
		}

		// The change is refused when the entry's account is no longer the one
		// the owner verified for: here the Discord account signed in for the
		// first time in between.
		late := model.WtDiscordIdentity{DiscordId: "730000000000000073", Username: "late"}
		lateEntry, err := model.WtAddAllowlistEntry(model.WtAllowlistInput{DiscordId: late.DiscordId}, now)
		require.NoError(t, err)
		verified, err := model.WtAllowlistStatusAccount(lateEntry.Id, model.WtAllowlistRemoved)
		require.NoError(t, err)
		assert.Zero(t, verified)
		require.NotNil(t, admitWtAccount(t, late, now).User)
		for _, accountID := range []int{verified, bystander.Id} {
			_, _, err = model.WtRemoveAllowlistEntry(lateEntry.Id, accountID, now)
			assert.ErrorIs(t, err, model.ErrWtAllowlistChanged)
		}
		lateListed := wtListedEntry(t, late.DiscordId)
		assert.Equal(t, model.WtAllowlistActive, lateListed.Status)
		assert.Equal(t, model.WtAccountEnabled, lateListed.AccountStatus)
	})
}

// A deleted member comes back only when the owner restores the entry, and then
// with a new, empty account.
func TestWtDeletedMemberReturnsOnlyThroughTheOwner(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		now := time.Now()
		_, owner := wtOwnerSession(t)
		identity := model.WtDiscordIdentity{DiscordId: "740000000000000074", Username: "voyager"}
		entry, err := model.WtAddAllowlistEntry(model.WtAllowlistInput{DiscordId: identity.DiscordId}, now)
		require.NoError(t, err)
		member := admitWtAccount(t, identity, now).User
		require.NotNil(t, member)
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", member.Id).Update("quota", 5000).Error)
		_, err = member.Delete()
		require.NoError(t, err)

		assert.Equal(t, &model.WtGateDecision{Gate: model.WtGateRemoved}, admitWtAccount(t, identity, now))
		listed := wtListedEntry(t, identity.DiscordId)
		assert.Equal(t, model.WtAllowlistRemoved, listed.Status)
		assert.Equal(t, model.WtAccountDeleted, listed.AccountStatus)
		assert.Zero(t, listed.UserId)

		// Restoring enables no account, so it takes no step-up verification.
		restored := decodeSecurityEnrollmentResponse(t, wtAllowlistStatus(WtRestoreAllowlistEntry, owner, "", entry.Id))
		require.True(t, restored.Success, restored.Message)
		listed = wtListedEntry(t, identity.DiscordId)
		assert.Equal(t, model.WtAllowlistActive, listed.Status)
		assert.Empty(t, listed.AccountStatus)

		// The next sign-in creates a new account with nothing of the old one;
		// the deleted account gives up the Discord ID and keeps its name.
		returning := admitWtAccount(t, identity, now).User
		require.NotNil(t, returning)
		assert.NotEqual(t, member.Id, returning.Id)
		assert.Zero(t, returning.Quota)
		assert.True(t, strings.HasPrefix(returning.Username, "discord_"), returning.Username)
		var deleted model.User
		require.NoError(t, model.DB.Unscoped().First(&deleted, member.Id).Error)
		assert.Empty(t, deleted.DiscordId)
		assert.Equal(t, 5000, deleted.Quota)
	})
}

func TestWtOwnerDiscordID(t *testing.T) {
	// The server and the `wt` commands refuse to start without a valid owner.
	for _, value := range []struct {
		id    string
		valid bool
	}{
		{"", false},
		{"owner", false},
		{"1000000000000000", false},
		{"100000000000000000000", false},
		{"10000000000000000a", false},
		{"10000000000000000", true},
		{"10000000000000000000", true},
		{" 100000000000000001 ", true},
	} {
		t.Setenv("WT_OWNER_DISCORD_ID", value.id)
		assert.Equal(t, value.valid, model.WtCheckOwnerDiscordID() == nil, value.id)
	}

	forEachWtDatabase(t, func(t *testing.T) {
		now := time.Now()

		// The owner signs in to root without an entry, and is never added to
		// or removed from the allowlist.
		_, err := model.WtAddAllowlistEntry(model.WtAllowlistInput{DiscordId: wtTestOwnerDiscordID}, now)
		assert.ErrorIs(t, err, model.ErrWtAllowlistOwner)
		request := model.WtEntryRequest{DiscordId: wtTestOwnerDiscordID, DiscordUsername: "tide", DisplayName: "tide", Status: model.WtEntryPending, RequestedAt: now.Unix()}
		require.NoError(t, model.DB.Create(&request).Error)
		_, err = model.WtApproveEntryRequest(request.Id, now)
		assert.ErrorIs(t, err, model.ErrWtAllowlistOwner)
		ownerID := wtTestOwnerDiscordID
		legacy := model.WtAllowlistEntry{DiscordId: &ownerID, Status: model.WtAllowlistActive, CreatedAt: now.Unix(), UpdatedAt: now.Unix()}
		require.NoError(t, model.DB.Create(&legacy).Error)
		_, err = model.WtAllowlistStatusAccount(legacy.Id, model.WtAllowlistRemoved)
		assert.ErrorIs(t, err, model.ErrWtOwnerRemoval)
		_, _, err = model.WtRemoveAllowlistEntry(legacy.Id, 0, now)
		assert.ErrorIs(t, err, model.ErrWtOwnerRemoval)
		require.NoError(t, model.DB.Delete(&legacy).Error)
		require.NoError(t, model.DB.Delete(&request).Error)

		// The root account holds access tokens, API keys, a password and a
		// second factor, which the owner's Discord sign-in asks for.
		owner, session := wtOwnerSession(t)
		pat, _ := createScopedAccessToken(t, owner.Id, 0, "profile:read")
		legacyToken := "wt-legacy-access-token-000000001"
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", owner.Id).Updates(map[string]any{"access_token": legacyToken, "password": "not-a-password-hash"}).Error)
		key := createWtAPIKey(t, owner.Id, common.TokenStatusEnabled)
		createWtAPIKey(t, owner.Id, common.TokenStatusDisabled)
		require.NoError(t, model.DB.Create(&model.TwoFA{UserId: owner.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}).Error)
		require.NoError(t, model.DB.Create(&model.TwoFABackupCode{UserId: owner.Id, CodeHash: "not-a-code-hash"}).Error)
		require.NoError(t, model.DB.Create(&model.PasskeyCredential{UserID: owner.Id, CredentialID: "wind-tide-test-credential", PublicKey: "AQID"}).Error)
		relay, dashboard := newWtRelay(), newAccessTokenTestRouter()
		require.Equal(t, http.StatusNoContent, wtRelayStatus(relay, key))
		for _, credential := range []string{pat, legacyToken} {
			require.Equal(t, http.StatusOK, wtDashboardStatus(dashboard, credential))
		}
		challenged := decodeSecurityEnrollmentResponse(t, wtDiscordSignIn(&oauth.OAuthUser{ProviderUserID: wtTestOwnerDiscordID, Username: "tide"}))
		require.True(t, challenged.Success, challenged.Message)
		assert.Contains(t, string(challenged.Data), `"require_verification":true`)

		// WT_OWNER_DISCORD_ID moves to another Discord account. Root is bound
		// to it once, and nothing the previous holder set up opens it.
		const nextOwnerID = "100000000000000099"
		t.Setenv("WT_OWNER_DISCORD_ID", nextOwnerID)
		for range 2 {
			require.NoError(t, model.WtBootstrapOwner())
		}
		rebound, err := model.WtOwnerAccount()
		require.NoError(t, err)
		assert.Equal(t, owner.Id, rebound.Id)
		assert.Equal(t, nextOwnerID, rebound.DiscordId)
		assert.Greater(t, rebound.AuthVersion, owner.AuthVersion)
		assert.Empty(t, rebound.Password)
		_, _, err = service.ValidateLoginSession(session)
		assert.Error(t, err)
		for _, credential := range []string{pat, legacyToken} {
			assert.Equal(t, http.StatusUnauthorized, wtDashboardStatus(dashboard, credential))
		}
		assert.Equal(t, http.StatusUnauthorized, wtRelayStatus(relay, key))
		for _, factor := range []any{&model.TwoFA{}, &model.TwoFABackupCode{}, &model.PasskeyCredential{}} {
			var left int64
			require.NoError(t, model.DB.Unscoped().Model(factor).Where("user_id = ?", owner.Id).Count(&left).Error)
			assert.Zero(t, left)
		}
		rebinds := wtAudits(t, "wt.owner_rebound")
		require.Len(t, rebinds, 1)
		assert.Equal(t, owner.Id, rebinds[0].UserId)
		assert.Equal(t, common.RoleRootUser, rebinds[0].ActorRole)
		assert.JSONEq(t, `{"access_tokens":2,"relay_keys":1,"passkeys":1,"two_factor":1}`, wtAuditParams(t, rebinds[0]))

		// The new owner signs in without the old second factor; the previous
		// Discord account is a stranger now.
		signedIn := decodeSecurityEnrollmentResponse(t, wtDiscordSignIn(&oauth.OAuthUser{ProviderUserID: nextOwnerID, Username: "tide"}))
		require.True(t, signedIn.Success, signedIn.Message)
		var login struct {
			AccessToken string `json:"access_token"`
			User        struct {
				Id int `json:"id"`
			} `json:"user"`
		}
		require.NoError(t, common.Unmarshal(signedIn.Data, &login))
		assert.NotEmpty(t, login.AccessToken)
		assert.Equal(t, owner.Id, login.User.Id)
		assert.Equal(t, model.WtEntryPending, admitWtAccount(t, model.WtDiscordIdentity{DiscordId: wtTestOwnerDiscordID, Username: "tide"}, now).Gate)

		// Root never signs in through an allowlist entry, not even one for its
		// own Discord ID while WT_OWNER_DISCORD_ID names another account.
		t.Setenv("WT_OWNER_DISCORD_ID", "100000000000000098")
		stale, err := model.WtAddAllowlistEntry(model.WtAllowlistInput{DiscordId: nextOwnerID}, now)
		require.NoError(t, err)
		_, err = model.WtAdmitDiscordAccount(model.WtDiscordIdentity{DiscordId: nextOwnerID, Username: "tide"}, now)
		assert.ErrorIs(t, err, model.ErrWtOwnerNotReady)
		_, err = model.WtAllowlistStatusAccount(stale.Id, model.WtAllowlistRemoved)
		assert.ErrorIs(t, err, model.ErrWtOwnerRemoval)

		// Another account that holds the configured ID stops the startup.
		createWtUser(t, model.User{Username: "holder", DiscordId: "100000000000000097"})
		t.Setenv("WT_OWNER_DISCORD_ID", "100000000000000097")
		assert.Error(t, model.WtBootstrapOwner())
		assert.Error(t, model.WtTakeOverDatabase())
	})
}

// A Discord sign-in finishes only in the browser that started it (login CSRF);
// a verification flow is bound to the session instead.
func TestWtDiscordSignInIsBoundToTheBrowser(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		owner, session := wtOwnerSession(t)
		discord := stubWtDiscord(t, wtTestOwnerDiscordID)
		login := wtRequest{body: `{"provider":"discord","intent":"login"}`}
		state, cookies := wtStartDiscord(t, login)
		require.Len(t, cookies, 1)
		assert.Equal(t, wtOAuthLoginCookie, cookies[0].Name)
		assert.Equal(t, "/api/oauth", cookies[0].Path)
		assert.True(t, cookies[0].HttpOnly)
		assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
		assert.Equal(t, int(oauthAuthFlowTTL/time.Second), cookies[0].MaxAge)

		// A callback without the cookie, with the cookie of another sign-in or
		// with a made-up one is refused before Discord is asked.
		_, otherCookies := wtStartDiscord(t, login)
		for name, sent := range map[string][]*http.Cookie{
			"no cookie":             nil,
			"another sign-in":       otherCookies,
			"made-up nonce":         {{Name: wtOAuthLoginCookie, Value: "made-up"}},
			"the digest as a nonce": {{Name: wtOAuthLoginCookie, Value: hex.EncodeToString(make([]byte, 32))}},
		} {
			refused := wtFinishDiscord(wtRequest{cookies: sent}, state)
			assert.Equal(t, http.StatusForbidden, refused.Code, name)
			assert.Equal(t, "State parameter is empty or mismatched", decodeSecurityEnrollmentResponse(t, refused).Message, name)
		}
		assert.Zero(t, discord.calls.Load())

		// The browser that started the sign-in finishes it, once, and its
		// cookie is cleared.
		finished := wtFinishDiscord(wtRequest{cookies: cookies}, state)
		signedIn := decodeSecurityEnrollmentResponse(t, finished)
		require.True(t, signedIn.Success, signedIn.Message)
		var started struct {
			AccessToken string `json:"access_token"`
			User        struct {
				Id int `json:"id"`
			} `json:"user"`
		}
		require.NoError(t, common.Unmarshal(signedIn.Data, &started))
		assert.NotEmpty(t, started.AccessToken)
		assert.Equal(t, owner.Id, started.User.Id)
		sent := finished.Result().Cookies()
		cleared := slices.IndexFunc(sent, func(cookie *http.Cookie) bool { return cookie.Name == wtOAuthLoginCookie })
		require.GreaterOrEqual(t, cleared, 0)
		assert.Negative(t, sent[cleared].MaxAge)
		assert.Equal(t, http.StatusForbidden, wtFinishDiscord(wtRequest{cookies: cookies}, state).Code)

		// The owner verifies with Discord for a step-up without the cookie, and
		// the proof removes a member.
		member := model.WtDiscordIdentity{DiscordId: "750000000000000075", Username: "deckhand"}
		entry, err := model.WtAddAllowlistEntry(model.WtAllowlistInput{DiscordId: member.DiscordId}, time.Now())
		require.NoError(t, err)
		account := admitWtAccount(t, member, time.Now()).User
		require.NotNil(t, account)
		verify := wtRequest{identity: session, role: common.RoleRootUser,
			body: fmt.Sprintf(`{"provider":"discord","intent":"verify","scope":"admin.user.manage","context":{"user_id":%d,"action":"disable"}}`, account.Id)}
		verifyState, verifyCookies := wtStartDiscord(t, verify)
		assert.Empty(t, verifyCookies)
		verified := decodeSecurityEnrollmentResponse(t, wtFinishDiscord(wtRequest{identity: session, role: common.RoleRootUser}, verifyState))
		require.True(t, verified.Success, verified.Message)
		var proof service.SecurityProof
		require.NoError(t, common.Unmarshal(verified.Data, &proof))
		removed := decodeSecurityEnrollmentResponse(t, wtAllowlistStatus(WtRemoveAllowlistEntry, session, proof.ProofToken, entry.Id))
		require.True(t, removed.Success, removed.Message)

		// Over HTTPS the cookie name carries the __Host- prefix: browsers keep
		// such a cookie only from a secure response, for the whole host and
		// no other, so neither a sibling subdomain nor a plain-HTTP response
		// can plant one. A cookie without the prefix no longer counts.
		previousSecure := common.SessionCookieSecure
		common.SessionCookieSecure = true
		t.Cleanup(func() { common.SessionCookieSecure = previousSecure })
		state, cookies = wtStartDiscord(t, login)
		require.Len(t, cookies, 1)
		assert.Equal(t, "__Host-wt_oauth_login", cookies[0].Name)
		assert.Equal(t, "/", cookies[0].Path)
		assert.Empty(t, cookies[0].Domain)
		assert.True(t, cookies[0].Secure)
		assert.True(t, cookies[0].HttpOnly)
		assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
		unprefixed := wtFinishDiscord(wtRequest{cookies: []*http.Cookie{{Name: "wt_oauth_login", Value: cookies[0].Value}}}, state)
		assert.Equal(t, http.StatusForbidden, unprefixed.Code)
		finished = wtFinishDiscord(wtRequest{cookies: cookies}, state)
		signedIn = decodeSecurityEnrollmentResponse(t, finished)
		require.True(t, signedIn.Success, signedIn.Message)
		sent = finished.Result().Cookies()
		cleared = slices.IndexFunc(sent, func(cookie *http.Cookie) bool { return cookie.Name == "__Host-wt_oauth_login" })
		require.GreaterOrEqual(t, cleared, 0)
		assert.Negative(t, sent[cleared].MaxAge)
		assert.Equal(t, "/", sent[cleared].Path)
		assert.True(t, sent[cleared].Secure)
	})
}

// The Discord binding is how a member signs in and how the allowlist finds the
// account, so no spelling of it can be cleared, even with the owner's
// verification.
func TestWtDiscordBindingIsNeverCleared(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		_, owner := wtOwnerSession(t)
		member := createWtUser(t, model.User{Username: "anchor", DiscordId: "760000000000000076", GitHubId: "760076"})
		id := strconv.Itoa(member.Id)
		for _, spelling := range []string{"discord", "Discord", " DISCORD ", "dİscord", "DİSCORD", "github"} {
			context, err := common.Marshal(service.AdminUserBindingContext{UserID: member.Id, BindingType: spelling})
			require.NoError(t, err)
			proof := issueSecurityEnrollmentProof(t, owner, service.VerificationOperation{Scope: service.VerificationScopeAdminUserBindingClear, Context: context}, service.VerificationMethodOAuth)
			response := decodeSecurityEnrollmentResponse(t, wtRequest{method: http.MethodDelete, target: "/api/user/" + id + "/bindings/" + url.PathEscape(spelling),
				identity: owner, role: common.RoleRootUser, proof: proof,
				params: gin.Params{{Key: "id", Value: id}, {Key: "binding_type", Value: spelling}}}.serve(AdminClearUserBinding))
			if spelling == "github" {
				assert.True(t, response.Success, response.Message)
				continue
			}
			assert.False(t, response.Success, spelling)
			assert.Equal(t, model.ErrWtDiscordOnly.Error(), response.Message, spelling)
		}
		account, err := model.GetUserById(member.Id, false)
		require.NoError(t, err)
		assert.Equal(t, "760000000000000076", account.DiscordId)
		assert.Empty(t, account.GitHubId)
	})
}

func TestWtEmergencyLogin(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		// An existing root account is bound to WT_OWNER_DISCORD_ID.
		root := createWtUser(t, model.User{Username: "root", Role: common.RoleRootUser})
		require.NoError(t, model.WtBootstrapOwner())
		owner, err := model.WtOwnerAccount()
		require.NoError(t, err)
		assert.Equal(t, root.Id, owner.Id)
		assert.Equal(t, wtTestOwnerDiscordID, owner.DiscordId)

		now := time.Now()
		token, err := model.IssueWtEmergencyToken(now)
		require.NoError(t, err)
		var stored []model.WtEmergencyToken
		require.NoError(t, model.DB.Find(&stored).Error)
		require.Len(t, stored, 1)
		digest := sha256.Sum256([]byte(token))
		assert.Equal(t, hex.EncodeToString(digest[:]), stored[0].TokenHash, "only the digest is stored")
		assert.Equal(t, now.Add(model.WtEmergencyTokenTTL).Unix(), stored[0].ExpiresAt)

		wrong := strings.Repeat("A", len(token))
		for _, attempt := range []struct {
			name, token string
			at          time.Time
		}{
			{"wrong token", wrong, now},
			{"truncated token", token[:len(token)-1], now},
			{"expired", token, now.Add(model.WtEmergencyTokenTTL)},
		} {
			assert.ErrorIs(t, model.ConsumeWtEmergencyToken(attempt.token, attempt.at), model.ErrWtEmergencyTokenInvalid, attempt.name)
		}
		require.NoError(t, model.ConsumeWtEmergencyToken(token, now.Add(model.WtEmergencyTokenTTL-time.Second)))
		assert.ErrorIs(t, model.ConsumeWtEmergencyToken(token, now), model.ErrWtEmergencyTokenInvalid, "replayed")

		older, err := model.IssueWtEmergencyToken(now)
		require.NoError(t, err)
		newer, err := model.IssueWtEmergencyToken(now)
		require.NoError(t, err)
		assert.ErrorIs(t, model.ConsumeWtEmergencyToken(older, now), model.ErrWtEmergencyTokenInvalid, "superseded")
		require.NoError(t, model.ConsumeWtEmergencyToken(newer, now))

		// The endpoint signs the owner in once and answers every failure alike.
		link, err := model.IssueWtEmergencyToken(time.Now())
		require.NoError(t, err)
		signedIn := wtServe(WtEmergencyLogin, http.MethodPost, "/api/wt/emergency-login", `{"token":"`+link+`"}`, 0)
		body := decodeSecurityEnrollmentResponse(t, signedIn)
		require.True(t, body.Success, body.Message)
		var login struct {
			AccessToken string `json:"access_token"`
			Session     struct {
				LoginMethod string `json:"login_method"`
			} `json:"session"`
			User struct {
				Id int `json:"id"`
			} `json:"user"`
		}
		require.NoError(t, common.Unmarshal(body.Data, &login))
		assert.NotEmpty(t, login.AccessToken)
		assert.Equal(t, "emergency", login.Session.LoginMethod)
		assert.Equal(t, owner.Id, login.User.Id)
		assert.NotEmpty(t, signedIn.Result().Cookies())
		assert.Equal(t, "no-store", signedIn.Header().Get("Cache-Control"))
		for _, attempt := range []string{`{"token":"` + link + `"}`, `{"token":"` + wrong + `"}`, `not json`} {
			refused := wtServe(WtEmergencyLogin, http.MethodPost, "/api/wt/emergency-login", attempt, 0)
			failure := decodeSecurityEnrollmentResponse(t, refused)
			assert.False(t, failure.Success, attempt)
			assert.Equal(t, "Emergency login link is invalid or has expired", failure.Message, attempt)
			assert.Empty(t, refused.Result().Cookies(), attempt)
		}

		// The audit trail records the emergency login and every failed
		// attempt, for root only, but never a token or its digest.
		assert.Len(t, wtAudits(t, "wt.emergency_login"), 1)
		failures := wtAudits(t, "wt.emergency_login_failed")
		require.Len(t, failures, 3)
		for _, failure := range failures {
			assert.Equal(t, owner.Id, failure.UserId)
			assert.Zero(t, failure.ActorRole)
			assert.False(t, failure.Success)
		}
		var audits []model.AuditLog
		require.NoError(t, model.LOG_DB.Find(&audits).Error)
		for _, audit := range audits {
			encoded, err := common.Marshal(audit)
			require.NoError(t, err)
			for _, secret := range []string{link, wrong} {
				secretDigest := sha256.Sum256([]byte(secret))
				assert.NotContains(t, string(encoded), secret)
				assert.NotContains(t, string(encoded), hex.EncodeToString(secretDigest[:]))
			}
		}

		// The link replaces Discord, not the owner's second factor: with one
		// enrolled it leads to the verification step, without a session.
		require.NoError(t, model.DB.Create(&model.TwoFA{UserId: owner.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}).Error)
		guarded, err := model.IssueWtEmergencyToken(time.Now())
		require.NoError(t, err)
		challenged := wtServe(WtEmergencyLogin, http.MethodPost, "/api/wt/emergency-login", `{"token":"`+guarded+`"}`, 0)
		challenge := decodeSecurityEnrollmentResponse(t, challenged)
		require.True(t, challenge.Success, challenge.Message)
		assert.Contains(t, string(challenge.Data), `"require_verification":true`)
		assert.NotContains(t, string(challenge.Data), "access_token")
		assert.Empty(t, challenged.Result().Cookies())
	})
}

func TestWtCheckin(t *testing.T) {
	beijingMidnight := time.Date(2026, 10, 14, 16, 0, 0, 0, time.UTC)
	assert.Equal(t, "2026-10-14", model.WtCheckinDay(beijingMidnight.Add(-time.Second)))
	assert.Equal(t, "2026-10-15", model.WtCheckinDay(beijingMidnight))
	assert.Equal(t, "2026-11", model.WtCheckinMonth(time.Date(2026, 10, 31, 16, 0, 0, 0, time.UTC)))

	forEachWtDatabase(t, func(t *testing.T) {
		now := beijingMidnight.Add(30 * time.Minute)
		pinWtClock(t, now)
		steady := createWtUser(t, model.User{Username: "steady"})
		lapsed := createWtUser(t, model.User{Username: "lapsed"})
		for _, record := range []struct {
			user *model.User
			date string
		}{
			{steady, "2026-10-12"}, {steady, "2026-10-13"}, {steady, "2026-10-14"},
			{lapsed, "2026-10-12"}, {lapsed, "2026-10-13"},
		} {
			require.NoError(t, model.DB.Create(&model.Checkin{UserId: record.user.Id, CheckinDate: record.date, QuotaAwarded: 1, CreatedAt: now.Unix()}).Error)
		}
		// Today is still open, so the streak runs to yesterday; a missed day
		// resets it.
		streak, err := model.WtCheckinStreak(steady.Id, now)
		require.NoError(t, err)
		assert.Equal(t, 3, streak)
		streak, err = model.WtCheckinStreak(lapsed.Id, now)
		require.NoError(t, err)
		assert.Zero(t, streak)
		require.NoError(t, model.DB.Create(&model.Checkin{UserId: steady.Id, CheckinDate: "2026-10-15", QuotaAwarded: 1, CreatedAt: now.Unix()}).Error)
		streak, err = model.WtCheckinStreak(steady.Id, now)
		require.NoError(t, err)
		assert.Equal(t, 4, streak)

		// Through the API: the fixed reward, the Beijing day and the streak.
		const reward = int64(4_999_500_000)
		member := createWtUser(t, model.User{Username: "member"})
		require.NoError(t, model.DB.Create(&model.Checkin{UserId: member.Id, CheckinDate: "2026-10-14", QuotaAwarded: 1, CreatedAt: now.Unix()}).Error)
		type checkinStatus struct {
			Today          string `json:"today"`
			CheckedInToday bool   `json:"checked_in_today"`
			Streak         int    `json:"streak"`
			RewardQuota    int64  `json:"reward_quota"`
		}
		var before checkinStatus
		statusBody := decodeSecurityEnrollmentResponse(t, wtServe(GetCheckinStatus, http.MethodGet, "/api/user/checkin", "", member.Id))
		require.True(t, statusBody.Success, statusBody.Message)
		require.NoError(t, common.Unmarshal(statusBody.Data, &before))
		assert.Equal(t, checkinStatus{Today: "2026-10-15", Streak: 1, RewardQuota: reward}, before)

		checkinBody := decodeSecurityEnrollmentResponse(t, wtServe(DoCheckin, http.MethodPost, "/api/user/checkin", "", member.Id))
		require.True(t, checkinBody.Success, checkinBody.Message)
		var result struct {
			QuotaAwarded int64  `json:"quota_awarded"`
			CheckinDate  string `json:"checkin_date"`
			Streak       int    `json:"streak"`
		}
		require.NoError(t, common.Unmarshal(checkinBody.Data, &result))
		assert.Equal(t, reward, result.QuotaAwarded)
		assert.Equal(t, "2026-10-15", result.CheckinDate)
		assert.Equal(t, 2, result.Streak)
		repeated := decodeSecurityEnrollmentResponse(t, wtServe(DoCheckin, http.MethodPost, "/api/user/checkin", "", member.Id))
		assert.False(t, repeated.Success)
		assert.Equal(t, "Already checked in today", repeated.Message)
		account, err := model.GetUserById(member.Id, false)
		require.NoError(t, err)
		assert.EqualValues(t, reward, account.Quota)

		var after checkinStatus
		statusBody = decodeSecurityEnrollmentResponse(t, wtServe(GetCheckinStatus, http.MethodGet, "/api/user/checkin", "", member.Id))
		require.NoError(t, common.Unmarshal(statusBody.Data, &after))
		assert.Equal(t, checkinStatus{Today: "2026-10-15", CheckedInToday: true, Streak: 2, RewardQuota: reward}, after)
	})
}

func TestWtLeaderboardAndPublicStats(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		require.NoError(t, model.WtBootstrapOwner())
		owner, err := model.WtOwnerAccount()
		require.NoError(t, err)
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", owner.Id).Update("request_count", 999).Error)
		alpha := createWtUser(t, model.User{Username: "alpha", DisplayName: "Alpha", DiscordId: "800000000000000001", Quota: 10, RequestCount: 100})
		bravo := createWtUser(t, model.User{Username: "bravo", DisplayName: "Bravo", Quota: 30, RequestCount: 50})
		charlie := createWtUser(t, model.User{Username: "charlie", DisplayName: "Charlie", Quota: 20, RequestCount: 50})
		delta := createWtUser(t, model.User{Username: "delta", RequestCount: 50})
		echo := createWtUser(t, model.User{Username: "echo", DisplayName: "Echo", Quota: 30})
		disabled := createWtUser(t, model.User{Username: "foxtrot", Status: common.UserStatusDisabled, Quota: 99, RequestCount: 999})
		admin := createWtUser(t, model.User{Username: "golf", Role: common.RoleAdminUser, Quota: 99, RequestCount: 999})
		dayStart := time.Date(2026, 10, 14, 16, 0, 0, 0, time.UTC) // 2026-10-15 00:00 in Beijing
		now := dayStart.Add(30 * time.Minute)
		pinWtClock(t, now)
		require.NoError(t, model.DB.Create(&model.WtDiscordProfile{DiscordId: alpha.DiscordId, Username: "alpha", GlobalName: "Alpha", AvatarHash: wtTestAvatarHash, UpdatedAt: now.Unix()}).Error)
		alphaAvatar := model.WtAvatarURL(alpha.DiscordId, wtTestAvatarHash)
		require.NotEmpty(t, alphaAvatar)

		var logs []model.Log
		for _, calls := range []struct {
			user    *model.User
			count   int
			at      time.Time
			logType int
		}{
			{alpha, 3, dayStart.Add(time.Hour), model.LogTypeConsume},
			{alpha, 10, dayStart.AddDate(0, 0, -6).Add(-time.Second), model.LogTypeConsume}, // before the week
			{bravo, 1, dayStart, model.LogTypeConsume},
			{bravo, 1, dayStart.Add(2 * time.Hour), model.LogTypeConsume},
			{charlie, 2, dayStart.Add(3 * time.Hour), model.LogTypeConsume},
			{charlie, 1, dayStart.Add(-time.Second), model.LogTypeConsume}, // yesterday in Beijing
			{delta, 1, dayStart.Add(4 * time.Hour), model.LogTypeConsume},
			{delta, 1, dayStart.AddDate(0, 0, -6), model.LogTypeConsume}, // first second of the week
			{echo, 4, dayStart.AddDate(0, 0, -3), model.LogTypeConsume},
			{echo, 3, dayStart.Add(time.Hour), model.LogTypeError},
			{disabled, 5, dayStart.Add(time.Hour), model.LogTypeConsume},
			{admin, 4, dayStart.Add(time.Hour), model.LogTypeConsume},
			{owner, 6, dayStart.Add(time.Hour), model.LogTypeConsume},
		} {
			for range calls.count {
				logs = append(logs, model.Log{UserId: calls.user.Id, Username: calls.user.Username, CreatedAt: calls.at.Unix(), Type: calls.logType})
			}
		}
		require.NoError(t, model.LOG_DB.Create(&logs).Error)
		require.NoError(t, model.DB.Create(&[]model.Checkin{
			{UserId: alpha.Id, CheckinDate: "2026-10-14", QuotaAwarded: 100, CreatedAt: now.Unix()},
			{UserId: bravo.Id, CheckinDate: "2026-10-15", QuotaAwarded: 250, CreatedAt: now.Unix()},
		}).Error)
		// The failed and streak boards read the call counters: filled once
		// from the logs in the order they were written, then moved by every
		// call; a second fill changes nothing.
		require.NoError(t, model.WtBackfillCallStats())
		require.NoError(t, model.WtRecordCall(bravo.Id, false, now))
		require.NoError(t, model.WtRecordCall(bravo.Id, true, now))
		require.NoError(t, model.WtBackfillCallStats())

		for _, board := range []struct {
			board, period string
			top           []model.WtLeaderboardItem
			echo          *model.WtLeaderboardRank
		}{
			{model.WtBoardCalls, model.WtPeriodToday, []model.WtLeaderboardItem{
				{Rank: 1, UserId: alpha.Id, DisplayName: "Alpha", AvatarURL: alphaAvatar, Value: 3},
				{Rank: 2, UserId: bravo.Id, DisplayName: "Bravo", Value: 2},
				{Rank: 2, UserId: charlie.Id, DisplayName: "Charlie", Value: 2},
				{Rank: 4, UserId: delta.Id, DisplayName: "delta", Value: 1},
			}, &model.WtLeaderboardRank{Rank: 5, Value: 0}},
			{model.WtBoardCalls, model.WtPeriodWeek, []model.WtLeaderboardItem{
				{Rank: 1, UserId: echo.Id, DisplayName: "Echo", Value: 4},
				{Rank: 2, UserId: alpha.Id, DisplayName: "Alpha", AvatarURL: alphaAvatar, Value: 3},
				{Rank: 2, UserId: charlie.Id, DisplayName: "Charlie", Value: 3},
				{Rank: 4, UserId: bravo.Id, DisplayName: "Bravo", Value: 2},
				{Rank: 4, UserId: delta.Id, DisplayName: "delta", Value: 2},
			}, &model.WtLeaderboardRank{Rank: 1, Value: 4}},
			{model.WtBoardCalls, model.WtPeriodAll, []model.WtLeaderboardItem{
				{Rank: 1, UserId: alpha.Id, DisplayName: "Alpha", AvatarURL: alphaAvatar, Value: 100},
				{Rank: 2, UserId: bravo.Id, DisplayName: "Bravo", Value: 50},
				{Rank: 2, UserId: charlie.Id, DisplayName: "Charlie", Value: 50},
				{Rank: 2, UserId: delta.Id, DisplayName: "delta", Value: 50},
			}, &model.WtLeaderboardRank{Rank: 5, Value: 0}},
			{model.WtBoardBalance, "", []model.WtLeaderboardItem{
				{Rank: 1, UserId: bravo.Id, DisplayName: "Bravo", Value: 30},
				{Rank: 1, UserId: echo.Id, DisplayName: "Echo", Value: 30},
				{Rank: 3, UserId: charlie.Id, DisplayName: "Charlie", Value: 20},
				{Rank: 4, UserId: alpha.Id, DisplayName: "Alpha", AvatarURL: alphaAvatar, Value: 10},
			}, &model.WtLeaderboardRank{Rank: 1, Value: 30}},
			{model.WtBoardFailed, model.WtPeriodToday, []model.WtLeaderboardItem{
				{Rank: 1, UserId: echo.Id, DisplayName: "Echo", Value: 3},
				{Rank: 2, UserId: bravo.Id, DisplayName: "Bravo", Value: 1},
			}, &model.WtLeaderboardRank{Rank: 1, Value: 3}},
			{model.WtBoardFailed, model.WtPeriodWeek, []model.WtLeaderboardItem{
				{Rank: 1, UserId: echo.Id, DisplayName: "Echo", Value: 3},
				{Rank: 2, UserId: bravo.Id, DisplayName: "Bravo", Value: 1},
			}, &model.WtLeaderboardRank{Rank: 1, Value: 3}},
			{model.WtBoardFailed, model.WtPeriodAll, []model.WtLeaderboardItem{
				{Rank: 1, UserId: echo.Id, DisplayName: "Echo", Value: 3},
				{Rank: 2, UserId: bravo.Id, DisplayName: "Bravo", Value: 1},
			}, &model.WtLeaderboardRank{Rank: 1, Value: 3}},
			// Streaks rank the best run; the run the member is on now rides
			// along. Echo's four calls came before the three failures.
			{model.WtBoardSuccessStreak, "", []model.WtLeaderboardItem{
				{Rank: 1, UserId: alpha.Id, DisplayName: "Alpha", AvatarURL: alphaAvatar, Value: 13, Current: 13},
				{Rank: 2, UserId: echo.Id, DisplayName: "Echo", Value: 4},
				{Rank: 3, UserId: charlie.Id, DisplayName: "Charlie", Value: 3, Current: 3},
				{Rank: 4, UserId: bravo.Id, DisplayName: "Bravo", Value: 2, Current: 1},
				{Rank: 4, UserId: delta.Id, DisplayName: "delta", Value: 2, Current: 2},
			}, &model.WtLeaderboardRank{Rank: 2, Value: 4}},
			{model.WtBoardFailStreak, "", []model.WtLeaderboardItem{
				{Rank: 1, UserId: echo.Id, DisplayName: "Echo", Value: 3, Current: 3},
				{Rank: 2, UserId: bravo.Id, DisplayName: "Bravo", Value: 1},
			}, &model.WtLeaderboardRank{Rank: 1, Value: 3}},
		} {
			built, err := model.BuildWtLeaderboard(board.board, board.period, now)
			require.NoError(t, err)
			assert.Equal(t, board.top, built.Top(), board.board+" "+board.period)
			assert.Equal(t, board.echo, built.Me(echo.Id), board.board+" "+board.period)
			for _, outsider := range []int{owner.Id, admin.Id, disabled.Id} {
				assert.Nil(t, built.Me(outsider), board.board+" "+board.period)
			}
		}

		stats, err := model.GetWtPublicStats(now)
		require.NoError(t, err)
		assert.Equal(t, model.WtPublicStats{Members: 5, CallsToday: 23, RewardedQuotaTotal: 350, UpdatedAt: now.Unix()}, *stats)

		// Only the first 50 ranks are listed; members tied on rank 50 all are.
		crowd := make([]model.User, 49)
		for i := range crowd {
			crowd[i] = model.User{Username: fmt.Sprintf("crowd%02d", i), Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: fmt.Sprintf("wt-crowd%02d", i), Quota: 1000}
		}
		require.NoError(t, model.DB.Create(&crowd).Error)
		crowded, err := model.BuildWtLeaderboard(model.WtBoardBalance, "", now)
		require.NoError(t, err)
		top := crowded.Top()
		require.Len(t, top, 51)
		assert.Equal(t, model.WtLeaderboardItem{Rank: 50, UserId: echo.Id, DisplayName: "Echo", Value: 30}, top[50])
		assert.Equal(t, &model.WtLeaderboardRank{Rank: 52, Value: 20}, crowded.Me(charlie.Id))

		// Over HTTP: me is null for the owner and boards are cached.
		served := func(userID int, target string) (bool, map[string]any) {
			body := decodeSecurityEnrollmentResponse(t, wtServe(WtGetLeaderboard, http.MethodGet, target, "", userID))
			var data map[string]any
			if body.Success {
				require.NoError(t, common.Unmarshal(body.Data, &data))
			}
			return body.Success, data
		}
		ok, ownerView := served(owner.Id, "/api/wt/leaderboard?board=balance")
		require.True(t, ok)
		assert.Equal(t, "balance", ownerView["board"])
		assert.Nil(t, ownerView["me"])
		assert.Contains(t, ownerView, "me")
		assert.Len(t, ownerView["items"], 51)
		createWtUser(t, model.User{Username: "latecomer", Quota: 5000})
		ok, charlieView := served(charlie.Id, "/api/wt/leaderboard?board=balance")
		require.True(t, ok)
		assert.Equal(t, ownerView["items"], charlieView["items"], "served from the cache")
		assert.Equal(t, map[string]any{"rank": float64(52), "value": float64(20)}, charlieView["me"])
		ok, defaults := served(charlie.Id, "/api/wt/leaderboard")
		require.True(t, ok)
		assert.Equal(t, "calls", defaults["board"])
		assert.Equal(t, "today", defaults["period"])
		ok, failedView := served(charlie.Id, "/api/wt/leaderboard?board=failed")
		require.True(t, ok)
		assert.Equal(t, "failed", failedView["board"])
		assert.Equal(t, "today", failedView["period"], "the failed board defaults to today")
		ok, streakView := served(charlie.Id, "/api/wt/leaderboard?board=success_streak&period=week")
		require.True(t, ok)
		assert.Equal(t, "success_streak", streakView["board"])
		assert.Equal(t, "", streakView["period"], "streaks have no period")
		streakItems, _ := streakView["items"].([]any)
		require.NotEmpty(t, streakItems)
		leader, _ := streakItems[0].(map[string]any)
		assert.Equal(t, float64(13), leader["current"], "the current run rides along")
		ok, _ = served(charlie.Id, "/api/wt/leaderboard?board=fail_streak")
		require.True(t, ok)
		for _, target := range []string{"/api/wt/leaderboard?board=calls&period=month", "/api/wt/leaderboard?board=failed&period=month", "/api/wt/leaderboard?board=credits"} {
			ok, _ := served(charlie.Id, target)
			assert.False(t, ok, target)
		}

		publicBody := decodeSecurityEnrollmentResponse(t, wtServe(WtGetPublicStats, http.MethodGet, "/api/wt/public-stats", "", 0))
		require.True(t, publicBody.Success, publicBody.Message)
		var public map[string]any
		require.NoError(t, common.Unmarshal(publicBody.Data, &public))
		assert.ElementsMatch(t, []string{"members", "calls_today", "rewarded_quota_total"}, slices.Collect(maps.Keys(public)))
		assert.Equal(t, float64(5+49+1), public["members"])
		assert.Equal(t, float64(350), public["rewarded_quota_total"])
		createWtUser(t, model.User{Username: "newest"})
		publicBody = decodeSecurityEnrollmentResponse(t, wtServe(WtGetPublicStats, http.MethodGet, "/api/wt/public-stats", "", 0))
		require.NoError(t, common.Unmarshal(publicBody.Data, &public))
		assert.Equal(t, float64(5+49+1), public["members"], "served from the cache")
	})
}

func TestWtDiscordOnlyGuard(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		reached := func(c *gin.Context) { c.Status(http.StatusNoContent) }
		engine := gin.New()
		api := engine.Group("/api")
		api.Use(middleware.WtDiscordOnly())
		for _, route := range []struct{ method, path string }{
			{http.MethodPost, "/setup"},
			{http.MethodGet, "/verification"},
			{http.MethodGet, "/reset_password"},
			{http.MethodPost, "/user/reset"},
			{http.MethodPost, "/user/register"},
			{http.MethodPost, "/user/login"},
			{http.MethodPost, "/user/login/2fa"},
			{http.MethodPost, "/user/login/verify"},
			{http.MethodPost, "/user/login/passkey/begin"},
			{http.MethodPost, "/user/passkey/login/begin"},
			{http.MethodPost, "/user/passkey/login/finish"},
			{http.MethodGet, "/oauth/wechat"},
			{http.MethodPost, "/oauth/wechat/bind"},
			{http.MethodGet, "/oauth/telegram/login"},
			{http.MethodPost, "/oauth/telegram/bind/start"},
			{http.MethodGet, "/oauth/telegram/bind/:flow_token"},
			{http.MethodGet, "/oauth/:provider"},
			{http.MethodDelete, "/user/:id/bindings/:binding_type"},
			{http.MethodPost, "/user/"},
			{http.MethodPut, "/user/"},
		} {
			api.Handle(route.method, route.path, reached)
		}
		api.POST("/oauth/state", GenerateOAuthCode)
		api.PUT("/user/self", func(c *gin.Context) {
			body, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			c.Data(http.StatusOK, "application/json", body)
		})

		bind, _, err := model.CreateAuthFlow(model.AuthFlowCreate{Purpose: model.AuthFlowPurposeOAuth, Provider: "discord", Intent: model.AuthFlowIntentBind, UserId: 1, Payload: "{}", ExpiresAt: time.Now().Add(time.Minute)})
		require.NoError(t, err)
		login, _, err := model.CreateAuthFlow(model.AuthFlowCreate{Purpose: model.AuthFlowPurposeOAuth, Provider: "discord", Intent: model.AuthFlowIntentLogin, Payload: "{}", ExpiresAt: time.Now().Add(time.Minute)})
		require.NoError(t, err)
		for _, request := range []struct {
			method, target, body string
			refused              bool
		}{
			{http.MethodPost, "/api/setup", `{}`, true},
			{http.MethodGet, "/api/verification?email=a@example.com", "", true},
			{http.MethodGet, "/api/reset_password?email=a@example.com", "", true},
			{http.MethodPost, "/api/user/reset", `{}`, true},
			{http.MethodPost, "/api/user/register", `{}`, true},
			{http.MethodPost, "/api/user/login", `{}`, true},
			{http.MethodPost, "/api/user/passkey/login/begin", `{}`, true},
			{http.MethodPost, "/api/user/passkey/login/finish", `{}`, true},
			{http.MethodGet, "/api/oauth/wechat?code=x", "", true},
			{http.MethodPost, "/api/oauth/wechat/bind", `{}`, true},
			{http.MethodGet, "/api/oauth/telegram/login", "", true},
			{http.MethodPost, "/api/oauth/telegram/bind/start", `{}`, true},
			{http.MethodGet, "/api/oauth/telegram/bind/flow", "", true},
			{http.MethodGet, "/api/oauth/github?code=x&state=y", "", true},
			{http.MethodGet, "/api/oauth/my-sso?code=x&state=y", "", true},
			{http.MethodGet, "/api/oauth/discord?code=x&state=" + bind, "", true},
			{http.MethodPost, "/api/oauth/state", `{"provider":"github","intent":"login"}`, true},
			{http.MethodPost, "/api/oauth/state", `{"provider":"my-sso","intent":"login"}`, true},
			{http.MethodPost, "/api/oauth/state", `{"provider":" discord ","intent":" bind "}`, true},
			{http.MethodPost, "/api/oauth/state", `not json`, true},
			{http.MethodDelete, "/api/user/7/bindings/Discord", "", true},
			// Unicode lower-casing turns these into "discord" as well, the way
			// AdminClearUserBinding reads the binding type.
			{http.MethodDelete, "/api/user/7/bindings/d%C4%B0scord", "", true},
			{http.MethodDelete, "/api/user/7/bindings/%20DISCORD%20", "", true},
			// No account has a password, so administrators create no accounts
			// and nobody sets a password, in any spelling of the key that
			// encoding/json matches to the password field.
			{http.MethodPost, "/api/user/", `{"username":"guest","password":"guest-password"}`, true},
			{http.MethodPut, "/api/user/self", `{"password":"new-password","original_password":""}`, true},
			{http.MethodPut, "/api/user/self", `{"Password":"new-password"}`, true},
			{http.MethodPut, "/api/user/self", `{"paſſword":"new-password"}`, true},
			{http.MethodPut, "/api/user/self", `{"password":"","PASSWORD":"new-password"}`, true},
			{http.MethodPut, "/api/user/self", `{"password":12345678}`, true},
			{http.MethodPut, "/api/user/self", `[{"password":"new-password"}]`, true},
			{http.MethodPut, "/api/user/", `{"id":7,"username":"member","password":"new-password"}`, true},
			{http.MethodPut, "/api/user/", `{"id":7,"username":"member","Password":"new-password"}`, true},
			{http.MethodPut, "/api/user/self", `{"display_name":"Tide"}`, false},
			{http.MethodPut, "/api/user/self", `{"display_name":"Tide","password":"","original_password":""}`, false},
			{http.MethodPut, "/api/user/self", `{"language":"zh","password":null}`, false},
			{http.MethodPut, "/api/user/", `{"id":7,"username":"member","quota":5,"password":""}`, false},
			// Second login factors, Discord sign-in and verification stay open.
			{http.MethodPost, "/api/user/login/2fa", `{}`, false},
			{http.MethodPost, "/api/user/login/verify", `{}`, false},
			{http.MethodPost, "/api/user/login/passkey/begin", `{}`, false},
			{http.MethodGet, "/api/oauth/discord?code=x&state=" + login, "", false},
			{http.MethodGet, "/api/oauth/discord?code=x&state=unknown", "", false},
			{http.MethodPost, "/api/oauth/state", `{"provider":"discord","intent":"verify"}`, false},
			{http.MethodDelete, "/api/user/7/bindings/github", "", false},
		} {
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(request.method, request.target, strings.NewReader(request.body)))
			name := request.method + " " + request.target + " " + request.body
			if request.refused {
				assert.Equal(t, http.StatusForbidden, recorder.Code, name)
				response := decodeSecurityEnrollmentResponse(t, recorder)
				assert.Equal(t, "WT_DISCORD_ONLY", response.Code, name)
				assert.Equal(t, middleware.WtDiscordOnlyMessage, response.Message, name)
				continue
			}
			assert.NotContains(t, recorder.Body.String(), "WT_DISCORD_ONLY", name)
			assert.NotEqual(t, http.StatusForbidden, recorder.Code, name)
		}

		// Discord sign-in still starts, with the request body intact.
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/oauth/state", strings.NewReader(`{"provider":"discord","intent":"login"}`)))
		response := decodeSecurityEnrollmentResponse(t, recorder)
		require.True(t, response.Success, response.Message)
		var started struct {
			FlowToken string `json:"flow_token"`
		}
		require.NoError(t, common.Unmarshal(response.Data, &started))
		flow, err := model.GetAuthFlow(started.FlowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth, Provider: "discord", Intent: model.AuthFlowIntentLogin})
		require.NoError(t, err)
		assert.Equal(t, model.AuthFlowIntentLogin, flow.Intent)
		oversized := httptest.NewRecorder()
		engine.ServeHTTP(oversized, httptest.NewRequest(http.MethodPost, "/api/oauth/state", strings.NewReader(strings.Repeat(" ", 64<<10)+`{"provider":"discord","intent":"login"}`)))
		assert.Equal(t, http.StatusRequestEntityTooLarge, oversized.Code)

		// A profile update reaches its handler with the body intact.
		profile := httptest.NewRecorder()
		engine.ServeHTTP(profile, httptest.NewRequest(http.MethodPut, "/api/user/self", strings.NewReader(`{"display_name":"Tide","password":""}`)))
		assert.Equal(t, http.StatusOK, profile.Code)
		assert.Equal(t, `{"display_name":"Tide","password":""}`, profile.Body.String())
	})
}

func TestWtDatabaseDefaults(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		previousName, previousPasswordLogin, previousPasswordRegister, previousRegister := common.SystemName, common.PasswordLoginEnabled, common.PasswordRegisterEnabled, common.RegisterEnabled
		previousGeneral := *operation_setting.GetGeneralSetting()
		previousLimit, previousMinutes, previousCount := setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes, setting.ModelRequestRateLimitCount
		common.OptionMapRWMutex.RLock()
		previousOptions := common.OptionMap
		common.OptionMapRWMutex.RUnlock()
		t.Cleanup(func() {
			common.SystemName, common.PasswordLoginEnabled, common.PasswordRegisterEnabled, common.RegisterEnabled = previousName, previousPasswordLogin, previousPasswordRegister, previousRegister
			*operation_setting.GetGeneralSetting() = previousGeneral
			setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes, setting.ModelRequestRateLimitCount = previousLimit, previousMinutes, previousCount
			common.OptionMapRWMutex.Lock()
			common.OptionMap = previousOptions
			common.OptionMapRWMutex.Unlock()
		})

		// Values stored before wind-tide took over the database are kept; the
		// others get wind-tide's defaults.
		require.NoError(t, model.DB.Create(&[]model.Option{{Key: "ModelRequestRateLimitCount", Value: "20"}, {Key: "SystemName", Value: "Old Site"}}).Error)
		for range 2 {
			require.NoError(t, model.WtPrepareDatabase())
		}
		system_setting.GetDiscordSettings().Enabled = false
		model.InitOptionMap()
		assert.Equal(t, "Old Site", common.SystemName)
		assert.False(t, common.PasswordLoginEnabled)
		assert.False(t, common.PasswordRegisterEnabled)
		assert.True(t, common.RegisterEnabled)
		assert.Zero(t, common.QuotaForNewUser)
		assert.True(t, system_setting.GetDiscordSettings().Enabled)
		assert.Equal(t, operation_setting.QuotaDisplayTypeUSD, operation_setting.GetGeneralSetting().QuotaDisplayType)
		assert.True(t, setting.ModelRequestRateLimitEnabled)
		assert.Equal(t, 1, setting.ModelRequestRateLimitDurationMinutes)
		assert.Equal(t, 20, setting.ModelRequestRateLimitCount)
		assert.Equal(t, operation_setting.CheckinSetting{Enabled: true, MinQuota: 4_999_500_000, MaxQuota: 4_999_500_000}, *operation_setting.GetCheckinSetting())

		// Visitors see only the opening, home, sign-in and about pages
		// (wind-tide/DESIGN.md §10): the model square and the model rankings
		// ask for a sign-in, and the upstream docs link is off.
		engine := gin.New()
		for _, module := range []string{"pricing", "rankings"} {
			engine.GET("/api/"+module, middleware.HeaderNavModuleAuth(module), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			visitor := httptest.NewRecorder()
			engine.ServeHTTP(visitor, httptest.NewRequest(http.MethodGet, "/api/"+module, nil))
			assert.Equal(t, http.StatusUnauthorized, visitor.Code, module)
		}
		var headerNav map[string]any
		require.NoError(t, common.UnmarshalJsonStr(common.OptionMap["HeaderNavModules"], &headerNav))
		assert.Equal(t, false, headerNav["docs"])
		assert.Equal(t, true, headerNav["about"])

		// The owner's root account exists, so the setup wizard never appears.
		_, err := model.WtOwnerAccount()
		require.NoError(t, err)
		constant.Setup = false
		model.CheckSetup()
		assert.True(t, constant.Setup)

		// Once wind-tide has taken over, the options are the owner's: one that
		// is gone is not written again.
		require.NoError(t, model.WtTakeOverDatabase())
		require.NoError(t, model.DB.Delete(&model.Option{Key: "PasswordLoginEnabled"}).Error)
		require.NoError(t, model.WtPrepareDatabase())
		var stored int64
		require.NoError(t, model.DB.Model(&model.Option{}).Where(&model.Option{Key: "PasswordLoginEnabled"}).Count(&stored).Error)
		assert.Zero(t, stored)
	})
}

// The first start of wind-tide on an existing database disables every account
// the owner never admitted, once.
func TestWtTakeOverDatabase(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		// A database used before wind-tide: root without Discord, a member
		// already on the allowlist, and accounts nobody admitted.
		root := createWtUser(t, model.User{Username: "root", Role: common.RoleRootUser})
		kept := createWtUser(t, model.User{Username: "kept", DiscordId: "770000000000000077"})
		_, err := model.WtAddAllowlistEntry(model.WtAllowlistInput{DiscordId: kept.DiscordId}, time.Now())
		require.NoError(t, err)
		outsider := createWtUser(t, model.User{Username: "outsider", DiscordId: "780000000000000078", Quota: 777})
		passwordOnly := createWtUser(t, model.User{Username: "password", Password: "not-a-password-hash"})
		admin := createWtUser(t, model.User{Username: "admin", Role: common.RoleAdminUser})
		banned := createWtUser(t, model.User{Username: "banned", DiscordId: "790000000000000079", Status: common.UserStatusDisabled})
		keptSession, outsiderSession := wtSession(t, kept.Id), wtSession(t, outsider.Id)
		keptKey, outsiderKey := createWtAPIKey(t, kept.Id, common.TokenStatusEnabled), createWtAPIKey(t, outsider.Id, common.TokenStatusEnabled)
		keptPAT, _ := createScopedAccessToken(t, kept.Id, 0, "profile:read")
		outsiderPAT, _ := createScopedAccessToken(t, outsider.Id, 0, "profile:read")
		legacyToken := "wt-legacy-access-token-000000002"
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", passwordOnly.Id).Update("access_token", legacyToken).Error)
		relay, dashboard := newWtRelay(), newAccessTokenTestRouter()
		require.Equal(t, http.StatusNoContent, wtRelayStatus(relay, outsiderKey))
		for _, credential := range []string{keptPAT, outsiderPAT, legacyToken} {
			require.Equal(t, http.StatusOK, wtDashboardStatus(dashboard, credential))
		}

		require.NoError(t, model.WtPrepareDatabase())
		require.NoError(t, model.WtTakeOverDatabase())
		owner, err := model.WtOwnerAccount()
		require.NoError(t, err)
		assert.Equal(t, root.Id, owner.Id)
		var accounts []model.User
		require.NoError(t, model.DB.Where("role <> ?", common.RoleRootUser).Find(&accounts).Error)
		statuses := map[string]int{}
		for _, account := range accounts {
			statuses[account.Username] = account.Status
		}
		assert.Equal(t, map[string]int{
			"kept":     common.UserStatusEnabled,
			"outsider": common.UserStatusDisabled,
			"password": common.UserStatusDisabled,
			"admin":    common.UserStatusDisabled,
			"banned":   common.UserStatusDisabled,
		}, statuses)
		// A disabled account loses its browser sessions and access tokens, and
		// its API keys are switched off, as when the owner removes a member.
		_, _, err = service.ValidateLoginSession(outsiderSession)
		assert.Error(t, err)
		_, _, err = service.ValidateLoginSession(keptSession)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, wtRelayStatus(relay, outsiderKey))
		assert.Equal(t, http.StatusNoContent, wtRelayStatus(relay, keptKey))
		for _, credential := range []string{outsiderPAT, legacyToken} {
			assert.Equal(t, http.StatusUnauthorized, wtDashboardStatus(dashboard, credential))
		}
		assert.Equal(t, http.StatusOK, wtDashboardStatus(dashboard, keptPAT))
		stored, err := model.GetUserById(passwordOnly.Id, true)
		require.NoError(t, err)
		assert.Empty(t, stored.Password)

		// Accounts with a Discord ID get a removed entry the owner can restore.
		for _, account := range []*model.User{outsider, banned} {
			listed := wtListedEntry(t, account.DiscordId)
			assert.Equal(t, model.WtAllowlistRemoved, listed.Status, account.Username)
			assert.Equal(t, "Removed by the wind-tide upgrade", listed.Note, account.Username)
			assert.Equal(t, account.Id, listed.UserId, account.Username)
			assert.Equal(t, model.WtAccountDisabled, listed.AccountStatus, account.Username)
		}
		takeovers := wtAudits(t, "wt.takeover")
		require.Len(t, takeovers, 1)
		assert.Equal(t, root.Id, takeovers[0].UserId)
		assert.JSONEq(t, `{"accounts":4,"removed_entries":2,"access_tokens":2,"relay_keys":1}`, wtAuditParams(t, takeovers[0]))
		var marks int64
		require.NoError(t, model.DB.Model(&model.Option{}).Where(&model.Option{Key: "WtAdoptedAt"}).Count(&marks).Error)
		assert.EqualValues(t, 1, marks)

		// The outsider is turned away at the gate. An account without an entry
		// can never be enabled, the outsider only by restoring its entry.
		assert.Equal(t, model.WtGateRemoved, admitWtAccount(t, model.WtDiscordIdentity{DiscordId: outsider.DiscordId, Username: "outsider"}, time.Now()).Gate)
		for _, account := range []*model.User{passwordOnly, admin} {
			stored, err := model.GetUserById(account.Id, false)
			require.NoError(t, err)
			stored.Status = common.UserStatusEnabled
			assert.ErrorIs(t, stored.Update(false), model.ErrWtAccountNotListed, account.Username)
		}
		outsiderEntry := wtListedEntry(t, outsider.DiscordId)
		accountID, err := model.WtAllowlistStatusAccount(outsiderEntry.Id, model.WtAllowlistActive)
		require.NoError(t, err)
		_, err = model.WtRestoreAllowlistEntry(outsiderEntry.Id, accountID, time.Now())
		require.NoError(t, err)
		// The access token stays deleted and the API key stays off until the
		// member switches it on again.
		assert.Equal(t, http.StatusUnauthorized, wtRelayStatus(relay, outsiderKey))
		assert.Equal(t, http.StatusUnauthorized, wtDashboardStatus(dashboard, outsiderPAT))

		// Later starts disable nobody, not even accounts created since. They
		// clear every password set in the meantime: a password would replace
		// Discord as the account's step-up verification.
		late := createWtUser(t, model.User{Username: "late"})
		require.NoError(t, model.DB.Model(&model.User{}).Where("id IN ?", []int{owner.Id, kept.Id}).Update("password", "not-a-password-hash").Error)
		ownerSession := wtSession(t, owner.Id)
		for range 2 {
			require.NoError(t, model.WtPrepareDatabase())
			require.NoError(t, model.WtTakeOverDatabase())
		}
		for _, account := range []*model.User{kept, outsider, late} {
			stored, err := model.GetUserById(account.Id, false)
			require.NoError(t, err)
			assert.Equal(t, common.UserStatusEnabled, stored.Status, account.Username)
		}
		restored, err := model.GetUserById(outsider.Id, false)
		require.NoError(t, err)
		assert.Equal(t, 777, restored.Quota)
		assert.Len(t, wtAudits(t, "wt.takeover"), 1)
		var withPassword int64
		require.NoError(t, model.DB.Unscoped().Model(&model.User{}).Where("password <> ?", "").Count(&withPassword).Error)
		assert.Zero(t, withPassword)
		requirements, err := service.GetVerificationRequirements(ownerSession, service.VerificationScopeAdminUserManage)
		require.NoError(t, err)
		require.Len(t, requirements.Methods, 1)
		assert.Equal(t, service.VerificationMethodOAuth, requirements.Methods[0].Method)
		assert.True(t, requirements.Methods[0].Available, requirements.Methods[0].Reason)
		cleared := wtAudits(t, "wt.passwords_cleared")
		require.Len(t, cleared, 2)
		assert.Equal(t, owner.Id, cleared[1].UserId)
		assert.JSONEq(t, `{"accounts":1}`, wtAuditParams(t, cleared[0]))
		assert.JSONEq(t, `{"accounts":2}`, wtAuditParams(t, cleared[1]))
	})
}

func TestWtModelRateLimitExemptsOwner(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		previousLimit, previousMinutes, previousCount := setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes, setting.ModelRequestRateLimitCount
		t.Cleanup(func() {
			setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes, setting.ModelRequestRateLimitCount = previousLimit, previousMinutes, previousCount
		})
		setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes, setting.ModelRequestRateLimitCount = true, 1, 2
		require.NoError(t, model.WtBootstrapOwner())
		owner, err := model.WtOwnerAccount()
		require.NoError(t, err)
		// The in-memory limiter lives as long as the test binary, so the member
		// gets an ID that no earlier run used.
		member := createWtUser(t, model.User{Id: 1_000_000 + int(time.Now().UnixNano()%1_000_000_000), Username: "member"})
		relay := newWtRelay()
		for _, account := range []struct {
			user     *model.User
			statuses []int
		}{
			{member, []int{http.StatusNoContent, http.StatusNoContent, http.StatusTooManyRequests}},
			{owner, []int{http.StatusNoContent, http.StatusNoContent, http.StatusNoContent, http.StatusNoContent}},
		} {
			key := createWtAPIKey(t, account.user.Id, common.TokenStatusEnabled)
			for i, status := range account.statuses {
				assert.Equal(t, status, wtRelayStatus(relay, key), "%s request %d", account.user.Username, i+1)
			}
		}
	})
}
