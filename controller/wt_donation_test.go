package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wind-tide: regression tests of donated Antigravity accounts (wind-tide/API.md
// §14) against a scripted stand-in for CPA's management API. The AI Studio
// pool and the pool page are covered in wt_pool_test.go.

// The CPA client sends the management key, treats redirects, 401s and
// oversized bodies as "pool unavailable" and recognizes a forgotten state.
func TestWtDonationCPAClientGuards(t *testing.T) {
	var seenAuth, mode atomic.Value
	mode.Store("wait")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth.Store(r.Header.Get("Authorization"))
		switch mode.Load().(string) {
		case "redirect":
			http.Redirect(w, r, "https://elsewhere.example/", http.StatusFound)
		case "unauthorized":
			w.WriteHeader(http.StatusUnauthorized)
		case "huge":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(bytes.Repeat([]byte("a"), (1<<20)+1))
		case "unknown-state":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"error","error":"unknown or expired state"}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"wait"}`))
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("WT_CPA_URL", server.URL)
	t.Setenv("WT_CPA_MANAGEMENT_KEY", "test-management-key")
	ctx := context.Background()

	status, _, err := service.WtCPAOAuthStatus(ctx, "state-1")
	require.NoError(t, err)
	assert.Equal(t, service.WtCPAOAuthWait, status)
	assert.Equal(t, "Bearer test-management-key", seenAuth.Load())

	mode.Store("unknown-state")
	status, _, err = service.WtCPAOAuthStatus(ctx, "state-1")
	require.NoError(t, err)
	assert.Equal(t, service.WtCPAOAuthUnknown, status)

	for _, failure := range []string{"redirect", "unauthorized", "huge"} {
		mode.Store(failure)
		_, _, err = service.WtCPAOAuthStatus(ctx, "state-1")
		assert.ErrorIs(t, err, service.ErrWtPoolUnavailable, failure)
	}

	t.Setenv("WT_CPA_MANAGEMENT_KEY", " ")
	assert.False(t, service.WtDonationEnabled())
	_, _, err = service.WtCPAOAuthStatus(ctx, "state-1")
	assert.ErrorIs(t, err, service.ErrWtPoolUnavailable)
	assert.Equal(t, "antigravity-someone@example.com.json", service.WtCredentialName(" someone@example.com "))
	assert.Equal(t, "ultra_lite", service.WtTierName("g1-ultra-lite-tier"))
	assert.Equal(t, "unknown", service.WtTierName("g2-mystery"))
}

// wtDonationRow reads a donation back without the owner filter.
func wtDonationRow(t *testing.T, id int) *model.WtDonation {
	t.Helper()
	var donation model.WtDonation
	require.NoError(t, model.DB.First(&donation, id).Error)
	return &donation
}

// The ledger counts only verifying and active donations against the limit,
// hides removed ones, never shows another member's donation, and settles a
// donation only while it is verifying.
func TestWtDonationLedgerQueries(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC))
		donor := createWtUser(t, model.User{Username: "donor"})
		other := createWtUser(t, model.User{Username: "other"})
		seed := func(userID int, email, status string, createdAt int64) int {
			donation := &model.WtDonation{UserId: userID, Provider: "antigravity", Email: email, Status: status, CredentialName: "antigravity-" + email + ".json", CreatedAt: createdAt, UpdatedAt: createdAt}
			require.NoError(t, model.DB.Create(donation).Error)
			return donation.Id
		}
		active := seed(donor.Id, "a@example.com", model.WtDonationActive, 100)
		verifying := seed(donor.Id, "b@example.com", model.WtDonationVerifying, 200)
		failed := seed(donor.Id, "c@example.com", model.WtDonationFailed, 300)
		removed := seed(donor.Id, "d@example.com", model.WtDonationRemoved, 400)
		seed(other.Id, "e@example.com", model.WtDonationActive, 500)

		// The same address donated to the AI Studio pool is another ledger:
		// it takes none of the Antigravity slots and claims nothing there.
		aistudio := &model.WtDonation{UserId: donor.Id, Provider: model.WtAisProvider, Email: "a@example.com", Status: model.WtDonationActive, CredentialName: "ais-acc-1", AuthIndex: "fp-1", CreatedAt: 600, UpdatedAt: 600}
		require.NoError(t, model.DB.Create(aistudio).Error)

		slots, err := model.CountWtDonationSlots(donor.Id, "antigravity")
		require.NoError(t, err)
		assert.EqualValues(t, 2, slots)
		slots, err = model.CountWtDonationSlots(donor.Id, model.WtAisProvider)
		require.NoError(t, err)
		assert.EqualValues(t, 1, slots)

		for email, expected := range map[string]bool{"a@example.com": true, "b@example.com": true, "c@example.com": false, "d@example.com": false, "e@example.com": true} {
			inPool, err := model.WtDonationEmailInPool(email, "antigravity", 0)
			require.NoError(t, err)
			assert.Equal(t, expected, inPool, email)
		}
		inPool, err := model.WtDonationEmailInPool("a@example.com", model.WtAisProvider, 0)
		require.NoError(t, err)
		assert.True(t, inPool)
		inPool, err = model.WtDonationEmailInPool("a@example.com", model.WtAisProvider, aistudio.Id)
		require.NoError(t, err)
		assert.False(t, inPool, "a donation does not conflict with itself")
		inPool, err = model.WtDonationEmailInPool("b@example.com", model.WtAisProvider, 0)
		require.NoError(t, err)
		assert.False(t, inPool, "an Antigravity donation is not in the AI Studio pool")

		claimed, err := model.WtDonationCredentialClaimed("antigravity-a@example.com.json", "antigravity", verifying)
		require.NoError(t, err)
		assert.True(t, claimed)
		claimed, err = model.WtDonationCredentialClaimed("antigravity-a@example.com.json", "antigravity", active)
		require.NoError(t, err)
		assert.False(t, claimed, "a donation does not conflict with itself")
		claimed, err = model.WtDonationCredentialClaimed("ais-acc-1", "antigravity", 0)
		require.NoError(t, err)
		assert.False(t, claimed, "pools do not share credential names")

		_, err = model.GetWtDonation(active, other.Id)
		assert.ErrorIs(t, err, model.ErrWtDonationNotFound)
		mine, err := model.GetWtDonation(active, donor.Id)
		require.NoError(t, err)
		assert.Equal(t, "a@example.com", mine.Email)

		listed, err := model.ListWtDonations(donor.Id, "antigravity")
		require.NoError(t, err)
		ids := make([]int, 0, len(listed))
		for _, donation := range listed {
			ids = append(ids, donation.Id)
		}
		assert.Equal(t, []int{failed, verifying, active}, ids, "newest first, removed hidden, other pools hidden")
		pending, err := model.ListVerifyingWtDonations("antigravity")
		require.NoError(t, err)
		require.Len(t, pending, 1)
		assert.Equal(t, verifying, pending[0].Id)

		assert.ErrorIs(t, model.FailWtDonation(removed, "late"), model.ErrWtDonationNotFound)
		assert.Equal(t, model.WtDonationRemoved, wtDonationRow(t, removed).Status)
		require.NoError(t, model.FailWtDonation(verifying, "Authorization timed out, start again"))
		row := wtDonationRow(t, verifying)
		assert.Equal(t, model.WtDonationFailed, row.Status)
		assert.Equal(t, "Authorization timed out, start again", row.FailReason)
		assert.Equal(t, "antigravity-b@example.com.json", row.CredentialName, "the file name survives a failure")

		for raw, want := range map[string]string{" Someone@Example.COM ": "someone@example.com", "a@b": "", "Name <a@b.c>": "", "": ""} {
			got, ok := model.WtNormalizeDonationEmail(raw)
			assert.Equal(t, want != "", ok, raw)
			assert.Equal(t, want, got, raw)
		}
	})
}

// The pool state of a credential: keywords for a dead token come before
// recovery times, because CPA also reports a quota cooldown as "error".
func TestWtDonationPoolStateMapping(t *testing.T) {
	now := time.Date(2026, 10, 15, 2, 0, 0, 0, time.UTC)
	in20 := now.Add(20 * time.Minute)
	in45 := now.Add(45 * time.Minute)
	past := now.Add(-time.Minute).Format(time.RFC3339)
	long := strings.Repeat("x", 230)
	cases := []struct {
		name    string
		entry   *service.WtCPACredential
		state   string
		retryAt int64
	}{
		{"missing", nil, service.WtPoolMissing, 0},
		{"disabled wins", &service.WtCPACredential{Status: "error", Disabled: true, StatusMessage: "invalid_grant"}, service.WtPoolDisabled, 0},
		{"unauthorized message", &service.WtCPACredential{Status: "active", StatusMessage: "Unauthorized: token revoked"}, service.WtPoolTokenInvalid, 0},
		{"invalid_grant with a cooldown is still a dead token", &service.WtCPACredential{Status: "error", StatusMessage: "invalid_grant", NextRetryAfter: in20.Format(time.RFC3339)}, service.WtPoolTokenInvalid, 0},
		{"401 in the message", &service.WtCPACredential{Status: "error", StatusMessage: "upstream returned 401"}, service.WtPoolTokenInvalid, 0},
		{"next_retry_after", &service.WtCPACredential{Status: "error", StatusMessage: "quota exhausted", NextRetryAfter: in20.Format(time.RFC3339)}, service.WtPoolCooldown, in20.Unix()},
		{"latest cooldown wins", &service.WtCPACredential{Status: "error", NextRetryAfter: in20.Format(time.RFC3339), Cooldowns: []service.WtCPACooldown{{RetryAt: in45.Format(time.RFC3339)}, {RetryAt: past}}}, service.WtPoolCooldown, in45.Unix()},
		{"unavailable without a time", &service.WtCPACredential{Status: "active", Unavailable: true}, service.WtPoolCooldown, 0},
		{"bare error", &service.WtCPACredential{Status: "error", StatusMessage: "refresh failed", NextRetryAfter: past}, service.WtPoolTokenInvalid, 0},
		{"refreshing", &service.WtCPACredential{Status: "refreshing"}, service.WtPoolOK, 0},
		{"pending", &service.WtCPACredential{Status: "pending"}, service.WtPoolOK, 0},
		{"active", &service.WtCPACredential{Status: "active", Success: 12, Failed: 1, StatusMessage: long}, service.WtPoolOK, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := service.WtPoolStateOf(tc.entry, now)
			assert.Equal(t, tc.state, info.State)
			assert.Equal(t, tc.retryAt, info.RetryAt)
			if tc.entry != nil {
				assert.Equal(t, tc.entry.Success, info.Success)
				assert.Equal(t, tc.entry.Failed, info.Failed)
				assert.LessOrEqual(t, len(info.Message), 200)
			}
		})
	}
}

const wtTestCPAKey = "test-management-key"

// wtDonationClock pins the Beijing day and the file times in these tests:
// 10:00 Beijing time on 2026-10-15.
var wtDonationClock = time.Date(2026, 10, 15, 2, 0, 0, 0, time.UTC)

// wtFakeCPA plays CPA for one test: it issues one state, answers the
// authorization status from a script, keeps a credential list and records
// the notes, deletions and Google calls it is asked for.
type wtFakeCPA struct {
	t      *testing.T
	server *httptest.Server
	mu     sync.Mutex

	state        string
	oauthReplies []string // "wait", "ok", "unknown" or "error:<reason>"; the last one repeats
	credentials  []map[string]any
	notes        map[string]string
	deleted      []string
	quotaBody    string
	quotaByIndex map[string]string // quota bodies per auth index, over quotaBody
	tierBody     string
	down         bool
	requests     []string
}

func newWtFakeCPA(t *testing.T) *wtFakeCPA {
	t.Helper()
	fake := &wtFakeCPA{
		t:            t,
		state:        "cpa-state-1",
		oauthReplies: []string{"ok"},
		notes:        map[string]string{},
		quotaBody:    `{"groups":[{"displayName":"Gemini 3 Pro","buckets":[{"displayName":"Gemini 3 Pro","window":"5h","remainingFraction":0.73,"resetTime":"2026-10-15T07:00:00Z"},{"displayName":"Gemini 3 Pro","window":"weekly","remainingFraction":0.5,"resetTime":"2026-10-20T00:00:00Z"},{"displayName":"No figure","window":"5h"}]}]}`,
		tierBody:     `{"currentTier":{"id":"free-tier"},"paidTier":{"id":"g1-pro-tier"}}`,
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	t.Setenv("WT_CPA_URL", fake.server.URL)
	t.Setenv("WT_CPA_MANAGEMENT_KEY", wtTestCPAKey)
	interval, deadline, inline := service.WtDonationVerifyInterval, service.WtDonationVerifyDeadline, service.WtDonationVerifySync
	service.WtDonationVerifyInterval, service.WtDonationVerifyDeadline, service.WtDonationVerifySync = time.Millisecond, 50*time.Millisecond, true
	t.Cleanup(func() {
		service.WtDonationVerifyInterval, service.WtDonationVerifyDeadline, service.WtDonationVerifySync = interval, deadline, inline
	})
	service.WtResetDonationCaches()
	t.Cleanup(service.WtResetDonationCaches)
	return fake
}

func (fake *wtFakeCPA) handle(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	route := r.Method + " " + r.URL.Path
	fake.requests = append(fake.requests, route)
	if fake.down {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+wtTestCPAKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch route {
	case "GET /v8/management/oauth/auth-url":
		fake.reply(w, http.StatusOK, map[string]any{"status": "ok", "url": "https://accounts.google.com/o/oauth2/v2/auth?client_id=test&state=" + fake.state, "state": fake.state})
	case "POST /v8/management/oauth/callback":
		var body map[string]string
		assert.NoError(fake.t, common.DecodeJson(r.Body, &body))
		assert.Equal(fake.t, "antigravity", body["provider"])
		assert.Equal(fake.t, fake.state, body["state"])
		assert.NotEmpty(fake.t, body["code"])
		fake.reply(w, http.StatusOK, map[string]any{"status": "ok"})
	case "GET /v8/management/oauth/status":
		answer := fake.oauthReplies[0]
		if len(fake.oauthReplies) > 1 {
			fake.oauthReplies = fake.oauthReplies[1:]
		}
		reason, failed := strings.CutPrefix(answer, "error:")
		switch {
		case answer == "unknown":
			fake.reply(w, http.StatusOK, map[string]any{"status": "error", "error": "unknown or expired state"})
		case failed:
			fake.reply(w, http.StatusOK, map[string]any{"status": "error", "error": reason})
		default:
			fake.reply(w, http.StatusOK, map[string]any{"status": answer})
		}
	case "GET /v8/management/credentials":
		files := []map[string]any{}
		name := r.URL.Query().Get("name")
		for _, entry := range fake.credentials {
			if name == "" || entry["name"] == name {
				files = append(files, entry)
			}
		}
		fake.reply(w, http.StatusOK, map[string]any{"files": files, "total": len(files), "page": 1, "page_size": 100, "has_more": false})
	case "PATCH /v8/management/credentials/fields":
		var body map[string]string
		assert.NoError(fake.t, common.DecodeJson(r.Body, &body))
		fake.notes[body["name"]] = body["note"]
		fake.reply(w, http.StatusOK, map[string]any{"status": "ok"})
	case "DELETE /v8/management/credentials":
		name := r.URL.Query().Get("name")
		fake.deleted = append(fake.deleted, name)
		kept := make([]map[string]any, 0, len(fake.credentials))
		for _, entry := range fake.credentials {
			if entry["name"] != name {
				kept = append(kept, entry)
			}
		}
		if len(kept) == len(fake.credentials) {
			fake.reply(w, http.StatusNotFound, map[string]any{"error": "auth file not found"})
			return
		}
		fake.credentials = kept
		fake.reply(w, http.StatusOK, map[string]any{"status": "ok"})
	case "POST /v8/management/requests/api-call":
		var body map[string]any
		assert.NoError(fake.t, common.DecodeJson(r.Body, &body))
		target, _ := body["url"].(string)
		header, _ := body["header"].(map[string]any)
		assert.Equal(fake.t, "Bearer $TOKEN$", header["Authorization"])
		assert.NotEmpty(fake.t, body["authIndex"])
		answer := fake.quotaBody
		if authIndex, _ := body["authIndex"].(string); fake.quotaByIndex[authIndex] != "" {
			answer = fake.quotaByIndex[authIndex]
		}
		if strings.Contains(target, "loadCodeAssist") {
			answer = fake.tierBody
		}
		fake.reply(w, http.StatusOK, map[string]any{"status_code": 200, "header": map[string][]string{}, "body": answer})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (fake *wtFakeCPA) reply(w http.ResponseWriter, status int, body any) {
	encoded, err := common.Marshal(body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

// addCredential puts a fresh Antigravity credential of email in the pool;
// fields override the defaults.
func (fake *wtFakeCPA) addCredential(email string, fields map[string]any) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	entry := map[string]any{
		"name":           "antigravity-" + email + ".json",
		"email":          email,
		"auth_index":     "idx-" + email,
		"project_id":     "proj-" + email,
		"provider":       "antigravity",
		"status":         "active",
		"status_message": "",
		"disabled":       false,
		"unavailable":    false,
		"updated_at":     model.WtNow().UTC().Format(time.RFC3339),
		"success":        12,
		"failed":         1,
	}
	maps.Copy(entry, fields)
	fake.credentials = append(fake.credentials, entry)
}

// requestsOf counts the calls of one route, "METHOD /path".
func (fake *wtFakeCPA) requestsOf(route string) int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	count := 0
	for _, seen := range fake.requests {
		if seen == route {
			count++
		}
	}
	return count
}

// wtDonationCall runs a donation handler for a signed-in member.
func wtDonationCall(t *testing.T, member service.AuthIdentity, handler gin.HandlerFunc, method, target, body string, params ...gin.Param) securityEnrollmentResponse {
	t.Helper()
	return decodeSecurityEnrollmentResponse(t, wtRequest{method: method, target: target, body: body, identity: member, params: params}.serve(handler))
}

type wtStartReply struct {
	FlowToken        string `json:"flow_token"`
	AuthorizationURL string `json:"authorization_url"`
	ExpiresAt        int64  `json:"expires_at"`
	Email            string `json:"email"`
}

// wtStartDonation starts a donation (donationID 0) or a re-authorization.
func wtStartDonation(t *testing.T, member service.AuthIdentity, email string, donationID int) wtStartReply {
	t.Helper()
	response := wtDonationCall(t, member, WtStartDonation, http.MethodPost, "/api/wt/donations/start", fmt.Sprintf(`{"email":%q,"donation_id":%d}`, email, donationID))
	require.True(t, response.Success, response.Message)
	var reply wtStartReply
	require.NoError(t, common.Unmarshal(response.Data, &reply))
	return reply
}

// Without both pool variables the feature is off: /api/status says so and
// every donation route answers the same message.
func TestWtDonationDisabledWithoutPool(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		t.Setenv("WT_CPA_URL", "")
		t.Setenv("WT_CPA_MANAGEMENT_KEY", "")
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)

		status := decodeSecurityEnrollmentResponse(t, wtServe(GetStatus, http.MethodGet, "/api/status", "", 0))
		var data map[string]any
		require.NoError(t, common.Unmarshal(status.Data, &data))
		assert.Equal(t, false, data["wt_donation_enabled"])

		response := wtDonationCall(t, session, WtStartDonation, http.MethodPost, "/api/wt/donations/start", `{"email":"someone@example.com","donation_id":0}`)
		assert.False(t, response.Success)
		assert.Equal(t, "Account donation is not enabled on this site", response.Message)
	})
}

// Starting a donation validates the address, snapshots the pool as digests,
// binds a five-minute flow to the member's session and returns CPA's Google
// link with the address pre-filled.
func TestWtDonationStartHandsOutTheGoogleLink(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		fake.addCredential("neighbour@example.com", nil)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)

		for _, bad := range []string{"", "nope", "a@b", "Name <a@b.c>", strings.Repeat("a", 250) + "@example.com"} {
			response := wtDonationCall(t, session, WtStartDonation, http.MethodPost, "/api/wt/donations/start", fmt.Sprintf(`{"email":%q}`, bad))
			assert.Equal(t, "Enter a valid email address", response.Message, bad)
		}
		response := wtDonationCall(t, session, WtStartDonation, http.MethodPost, "/api/wt/donations/start", `{"email":`)
		assert.Equal(t, "Invalid parameters", response.Message)

		started := wtStartDonation(t, session, " Someone@Example.COM ", 0)
		assert.Equal(t, "someone@example.com", started.Email)
		link, err := url.Parse(started.AuthorizationURL)
		require.NoError(t, err)
		assert.Equal(t, "accounts.google.com", link.Host)
		assert.Equal(t, "someone@example.com", link.Query().Get("login_hint"))
		assert.Equal(t, fake.state, link.Query().Get("state"))
		assert.InDelta(t, time.Now().Add(5*time.Minute).Unix(), started.ExpiresAt, 5)

		flow, err := model.GetAuthFlow(started.FlowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeWtDonation, Provider: "antigravity", Intent: model.AuthFlowIntentWtDonate, UserId: member.Id, SessionId: session.SessionID})
		require.NoError(t, err)
		var payload struct {
			CPAState   string   `json:"cpa_state"`
			Email      string   `json:"email"`
			DonationID int      `json:"donation_id"`
			StartedAt  int64    `json:"started_at"`
			PoolBefore []string `json:"pool_before"`
		}
		require.NoError(t, common.UnmarshalJsonStr(flow.Payload, &payload))
		assert.Equal(t, fake.state, payload.CPAState)
		assert.Equal(t, "someone@example.com", payload.Email)
		assert.Zero(t, payload.DonationID)
		assert.Equal(t, wtDonationClock.Unix(), payload.StartedAt)
		digest := sha256.Sum256([]byte("antigravity-neighbour@example.com.json"))
		assert.Equal(t, []string{hex.EncodeToString(digest[:])}, payload.PoolBefore)
		assert.NotContains(t, flow.Payload, "neighbour", "the snapshot stores digests, not other people's addresses")
	})
}

// An account already donated by anyone, or already in the pool under any
// spelling, is refused before Google is involved.
func TestWtDonationRejectsAccountsAlreadyInPool(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		other := createWtUser(t, model.User{Username: "other"})
		session := wtSession(t, member.Id)
		require.NoError(t, model.DB.Create(&model.WtDonation{UserId: other.Id, Provider: "antigravity", Email: "taken@example.com", Status: model.WtDonationActive, CredentialName: "antigravity-taken@example.com.json"}).Error)
		fake.addCredential("pooled@example.com", nil)
		fake.addCredential("Mixed@Example.com", nil)

		for _, email := range []string{"taken@example.com", "pooled@example.com", "mixed@example.com"} {
			response := wtDonationCall(t, session, WtStartDonation, http.MethodPost, "/api/wt/donations/start", fmt.Sprintf(`{"email":%q}`, email))
			assert.False(t, response.Success, email)
			assert.Equal(t, "This account is already in the pool", response.Message, email)
		}
		assert.Zero(t, fake.requestsOf("GET /v8/management/oauth/auth-url"))
	})
}

// Ten verifying or active donations fill a member's slots; failed and
// removed ones do not count.
func TestWtDonationLimitsEachMemberToTenSlots(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		for i := range 10 {
			status := model.WtDonationActive
			if i%2 == 1 {
				status = model.WtDonationVerifying
			}
			require.NoError(t, model.DB.Create(&model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: fmt.Sprintf("slot%d@example.com", i), Status: status}).Error)
		}
		require.NoError(t, model.DB.Create(&model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: "old@example.com", Status: model.WtDonationFailed}).Error)
		require.NoError(t, model.DB.Create(&model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: "gone@example.com", Status: model.WtDonationRemoved}).Error)

		response := wtDonationCall(t, session, WtStartDonation, http.MethodPost, "/api/wt/donations/start", `{"email":"eleventh@example.com"}`)
		assert.False(t, response.Success)
		assert.Equal(t, "You can donate at most 10 accounts", response.Message)
		assert.Equal(t, "You can donate at most {{max}} accounts", response.MessageKey)

		require.NoError(t, model.DB.Model(&model.WtDonation{}).Where("email = ?", "slot0@example.com").Update("status", model.WtDonationRemoved).Error)
		wtStartDonation(t, session, "eleventh@example.com", 0)
	})
}

// wtCallbackAddress is the address Google sends the member's browser to.
func wtCallbackAddress(state string) string {
	return "http://localhost:51121/oauth-callback?state=" + url.QueryEscape(state) + "&code=4%2F0Atest-code&scope=email"
}

// wtSubmitCallback pastes an address back for a started donation.
func wtSubmitCallback(t *testing.T, member service.AuthIdentity, flowToken, address string) securityEnrollmentResponse {
	t.Helper()
	body, err := common.Marshal(map[string]string{"flow_token": flowToken, "callback_url": address})
	require.NoError(t, err)
	return wtDonationCall(t, member, WtSubmitDonationCallback, http.MethodPost, "/api/wt/donations/callback", string(body))
}

// wtDonationIDOf reads the id of an accepted callback.
func wtDonationIDOf(t *testing.T, response securityEnrollmentResponse) int {
	t.Helper()
	require.True(t, response.Success, response.Message)
	var reply struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
	}
	require.NoError(t, common.Unmarshal(response.Data, &reply))
	assert.Equal(t, model.WtDonationVerifying, reply.Status)
	return reply.ID
}

// A pasted address reaches CPA as state and code only, the flow is spent,
// and the donation is bound to the credential CPA wrote for the address.
func TestWtDonationCallbackBindsTheCredential(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		started := wtStartDonation(t, session, "someone@example.com", 0)
		// CPA writes the file while it exchanges the code; here it is there
		// before the paste, which the verifier cannot tell apart.
		fake.addCredential("someone@example.com", nil)

		id := wtDonationIDOf(t, wtSubmitCallback(t, session, started.FlowToken, wtCallbackAddress(fake.state)))

		assert.Equal(t, 1, fake.requestsOf("POST /v8/management/oauth/callback"))
		row := wtDonationRow(t, id)
		assert.Equal(t, member.Id, row.UserId)
		assert.Equal(t, model.WtDonationActive, row.Status)
		assert.Equal(t, "antigravity-someone@example.com.json", row.CredentialName)
		assert.Equal(t, "idx-someone@example.com", row.AuthIndex)
		assert.Equal(t, "proj-someone@example.com", row.ProjectId)
		assert.Equal(t, wtDonationClock.Unix(), row.VerifiedAt)
		assert.Equal(t, wtDonationClock.Unix(), row.SubmittedAt)
		assert.Empty(t, row.CpaState)
		assert.Empty(t, row.FailReason)
		assert.Equal(t, fmt.Sprintf("donor:%d", member.Id), fake.notes["antigravity-someone@example.com.json"])
		_, err := model.GetAuthFlow(started.FlowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeWtDonation})
		assert.ErrorIs(t, err, model.ErrAuthFlowConsumed)
		events, err := model.WtListPoolEvents(0, 10)
		require.NoError(t, err)
		require.Len(t, events, 1, "the pool page announces the donation")
		assert.Equal(t, model.WtPoolEventAntigravityDonated, events[0].Kind)
		assert.Equal(t, "antigravity", events[0].Provider)
		assert.Equal(t, "donor", events[0].DisplayName)
		assert.Equal(t, 1, events[0].Amount)
		assert.Equal(t, wtDonationClock.Unix(), events[0].CreatedAt)
	})
}

// Each way CPA can report a failed authorization leaves its own reason on
// the donation, and a session that never finishes times out at the deadline.
func TestWtDonationVerificationFailureReasons(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		for i, tc := range []struct {
			replies []string
			reason  string
		}{
			{[]string{"wait", "error:authorization timed out"}, service.WtDonationFailTimeout},
			{[]string{"error:token exchange failed"}, service.WtDonationFailDenied},
			{[]string{"unknown"}, service.WtDonationFailUnknown},
			{[]string{"wait"}, service.WtDonationFailTimeout},
		} {
			fake.mu.Lock()
			fake.oauthReplies = tc.replies
			fake.mu.Unlock()
			email := fmt.Sprintf("case%d@example.com", i)
			started := wtStartDonation(t, session, email, 0)
			id := wtDonationIDOf(t, wtSubmitCallback(t, session, started.FlowToken, wtCallbackAddress(fake.state)))
			row := wtDonationRow(t, id)
			assert.Equal(t, model.WtDonationFailed, row.Status, email)
			assert.Equal(t, tc.reason, row.FailReason, email)
			assert.Empty(t, row.CpaState, email)
		}
	})
}

// When CPA is down at the paste, the donation fails with the pool message
// and the member starts over instead of resubmitting the spent flow.
func TestWtDonationCallbackWhenThePoolIsDown(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		started := wtStartDonation(t, session, "someone@example.com", 0)
		fake.mu.Lock()
		fake.down = true
		fake.mu.Unlock()

		response := wtSubmitCallback(t, session, started.FlowToken, wtCallbackAddress(fake.state))
		assert.False(t, response.Success)
		assert.Equal(t, "The account pool is temporarily unavailable, try again later", response.Message)
		var rows []model.WtDonation
		require.NoError(t, model.DB.Where("user_id = ?", member.Id).Find(&rows).Error)
		require.Len(t, rows, 1)
		assert.Equal(t, model.WtDonationFailed, rows[0].Status)
		assert.Equal(t, service.WtDonationFailPool, rows[0].FailReason)
		_, err := model.GetAuthFlow(started.FlowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeWtDonation})
		assert.ErrorIs(t, err, model.ErrAuthFlowConsumed)
	})
}

// An address of another authorization, or an incomplete one, is refused
// without spending the flow, so the member can paste the right one.
func TestWtDonationCallbackOfAnotherAuthorizationKeepsTheFlow(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		started := wtStartDonation(t, session, "someone@example.com", 0)
		fake.addCredential("someone@example.com", nil)

		response := wtSubmitCallback(t, session, started.FlowToken, wtCallbackAddress("somebody-elses-state"))
		assert.False(t, response.Success)
		assert.Equal(t, "This address does not belong to the current donation", response.Message)

		for _, address := range []string{
			"",
			"not an address at all",
			"http://localhost:51121/oauth-callback?scope=email",
			"http://localhost:51121/oauth-callback?state=" + fake.state,
			"http://localhost:51121/oauth-callback?" + strings.Repeat("x=1&", 600) + "state=" + fake.state,
		} {
			response = wtSubmitCallback(t, session, started.FlowToken, address)
			assert.False(t, response.Success, address)
			assert.Equal(t, "Paste the complete address from the browser's address bar", response.Message, address)
		}
		assert.Zero(t, fake.requestsOf("POST /v8/management/oauth/callback"))

		wtDonationIDOf(t, wtSubmitCallback(t, session, started.FlowToken, wtCallbackAddress(fake.state)))
	})
}

// A flow belongs to one member in one browser session for five minutes and
// can be spent once; everything else is "invalid or expired".
func TestWtDonationCallbackRejectsInvalidFlows(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		stranger := createWtUser(t, model.User{Username: "stranger"})
		session := wtSession(t, member.Id)
		address := wtCallbackAddress(fake.state)
		const invalid = "This donation session is invalid or has expired, start again"

		started := wtStartDonation(t, session, "a@example.com", 0)
		response := wtSubmitCallback(t, wtSession(t, member.Id), started.FlowToken, address)
		assert.Equal(t, invalid, response.Message, "another browser session of the same member")
		response = wtSubmitCallback(t, wtSession(t, stranger.Id), started.FlowToken, address)
		assert.Equal(t, invalid, response.Message, "another member")
		response = wtSubmitCallback(t, session, "not-a-token", address)
		assert.Equal(t, invalid, response.Message, "an unknown token")

		require.NoError(t, model.DB.Model(&model.AuthFlow{}).Where("purpose = ?", model.AuthFlowPurposeWtDonation).Update("expires_at", time.Now().Add(-time.Minute)).Error)
		response = wtSubmitCallback(t, session, started.FlowToken, address)
		assert.Equal(t, invalid, response.Message, "expired")

		started = wtStartDonation(t, session, "b@example.com", 0)
		fake.addCredential("b@example.com", nil)
		wtDonationIDOf(t, wtSubmitCallback(t, session, started.FlowToken, address))
		response = wtSubmitCallback(t, session, started.FlowToken, address)
		assert.Equal(t, invalid, response.Message, "replay")
		assert.Equal(t, 1, fake.requestsOf("POST /v8/management/oauth/callback"))
	})
}

// An address carrying Google's error spends the flow and creates nothing.
func TestWtDonationCallbackWithGoogleErrorSpendsTheFlow(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		started := wtStartDonation(t, session, "someone@example.com", 0)

		response := wtSubmitCallback(t, session, started.FlowToken, "http://localhost:51121/oauth-callback?state="+fake.state+"&error=access_denied")
		assert.False(t, response.Success)
		assert.Equal(t, "Google denied the authorization", response.Message)
		var count int64
		require.NoError(t, model.DB.Model(&model.WtDonation{}).Count(&count).Error)
		assert.Zero(t, count)
		_, err := model.GetAuthFlow(started.FlowToken, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeWtDonation})
		assert.ErrorIs(t, err, model.ErrAuthFlowConsumed)
		assert.Zero(t, fake.requestsOf("POST /v8/management/oauth/callback"))
	})
}

// When the member signs in to Google with another account than typed, the
// one new credential since the snapshot is bound and the ledger follows it;
// two new credentials cannot be told apart and fail the donation.
func TestWtDonationBindsTheAccountGoogleActuallyAuthorized(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		fake.addCredential("neighbour@example.com", nil)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)

		started := wtStartDonation(t, session, "typed@example.com", 0)
		fake.addCredential("actual@example.com", nil)
		id := wtDonationIDOf(t, wtSubmitCallback(t, session, started.FlowToken, wtCallbackAddress(fake.state)))
		row := wtDonationRow(t, id)
		assert.Equal(t, model.WtDonationActive, row.Status)
		assert.Equal(t, "actual@example.com", row.Email)
		assert.Equal(t, "antigravity-actual@example.com.json", row.CredentialName)
		assert.Equal(t, "idx-actual@example.com", row.AuthIndex)

		started = wtStartDonation(t, session, "typed2@example.com", 0)
		fake.addCredential("first@example.com", nil)
		fake.addCredential("second@example.com", nil)
		id = wtDonationIDOf(t, wtSubmitCallback(t, session, started.FlowToken, wtCallbackAddress(fake.state)))
		row = wtDonationRow(t, id)
		assert.Equal(t, model.WtDonationFailed, row.Status)
		assert.Equal(t, service.WtDonationFailMismatch, row.FailReason)
		assert.Equal(t, "typed2@example.com", row.Email)
	})
}

// After a restart the verifying donations are settled once by address: the
// pool either has the file by now or it never will.
func TestWtDonationStartupSettlesVerifyingRows(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		fake.addCredential("arrived@example.com", nil)
		arrived := &model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: "arrived@example.com", Status: model.WtDonationVerifying, CpaState: "lost-1", StartedAt: wtDonationClock.Unix()}
		missing := &model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: "missing@example.com", Status: model.WtDonationVerifying, CpaState: "lost-2", StartedAt: wtDonationClock.Unix()}
		require.NoError(t, model.DB.Create(arrived).Error)
		require.NoError(t, model.DB.Create(missing).Error)

		service.WtSettleVerifyingDonations()

		row := wtDonationRow(t, arrived.Id)
		assert.Equal(t, model.WtDonationActive, row.Status)
		assert.Equal(t, "idx-arrived@example.com", row.AuthIndex)
		assert.Empty(t, row.CpaState)
		row = wtDonationRow(t, missing.Id)
		assert.Equal(t, model.WtDonationFailed, row.Status)
		assert.Equal(t, service.WtDonationFailUnknown, row.FailReason)
	})
}

// Re-authorizing skips the duplicate checks, keeps the record and its id,
// and is only allowed for active and failed donations.
func TestWtDonationReauthorizationReusesTheRecord(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		fake.addCredential("someone@example.com", map[string]any{"status": "error", "status_message": "invalid_grant: Token has been expired or revoked."})
		existing := &model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: "someone@example.com", Status: model.WtDonationActive, CredentialName: "antigravity-someone@example.com.json", AuthIndex: "idx-someone@example.com", ProjectId: "proj-someone@example.com", VerifiedAt: 1}
		require.NoError(t, model.DB.Create(existing).Error)

		response := wtDonationCall(t, session, WtStartDonation, http.MethodPost, "/api/wt/donations/start", `{"email":"someone@example.com"}`)
		assert.Equal(t, "This account is already in the pool", response.Message, "a new donation of the same account is refused")

		started := wtStartDonation(t, session, "ignored@example.com", existing.Id)
		assert.Equal(t, "someone@example.com", started.Email, "the record's address wins over the body")
		id := wtDonationIDOf(t, wtSubmitCallback(t, session, started.FlowToken, wtCallbackAddress(fake.state)))
		assert.Equal(t, existing.Id, id)
		row := wtDonationRow(t, id)
		assert.Equal(t, model.WtDonationActive, row.Status)
		assert.Equal(t, wtDonationClock.Unix(), row.VerifiedAt)
		var count int64
		require.NoError(t, model.DB.Model(&model.WtDonation{}).Where("user_id = ?", member.Id).Count(&count).Error)
		assert.EqualValues(t, 1, count)

		require.NoError(t, model.DB.Model(&model.WtDonation{}).Where("id = ?", id).Update("status", model.WtDonationVerifying).Error)
		response = wtDonationCall(t, session, WtStartDonation, http.MethodPost, "/api/wt/donations/start", fmt.Sprintf(`{"donation_id":%d}`, id))
		assert.Equal(t, "Donation does not exist", response.Message, "a verifying donation cannot be re-authorized")
		other := createWtUser(t, model.User{Username: "other"})
		response = wtDonationCall(t, wtSession(t, other.Id), WtStartDonation, http.MethodPost, "/api/wt/donations/start", fmt.Sprintf(`{"donation_id":%d}`, id))
		assert.Equal(t, "Donation does not exist", response.Message, "somebody else's donation")
	})
}

// wtListReply is the shape of GET /api/wt/donations the tests read.
type wtListReply struct {
	Max   int `json:"max"`
	Items []struct {
		ID         int                 `json:"id"`
		Provider   string              `json:"provider"`
		Email      string              `json:"email"`
		Status     string              `json:"status"`
		FailReason string              `json:"fail_reason"`
		Pool       *service.WtPoolInfo `json:"pool"`
		LastTest   *struct {
			At        int64  `json:"at"`
			OK        bool   `json:"ok"`
			LatencyMs int    `json:"latency_ms"`
			Message   string `json:"message"`
		} `json:"last_test"`
		CreatedAt  int64 `json:"created_at"`
		VerifiedAt int64 `json:"verified_at"`
	} `json:"items"`
}

// wtListDonations reads the member's list.
func wtListDonations(t *testing.T, member service.AuthIdentity) wtListReply {
	t.Helper()
	response := wtDonationCall(t, member, WtListDonations, http.MethodGet, "/api/wt/donations", "")
	require.True(t, response.Success, response.Message)
	var reply wtListReply
	require.NoError(t, common.Unmarshal(response.Data, &reply))
	return reply
}

// wtActiveDonation seeds an active donation bound to the pool credential the
// fake would hold for email.
func wtActiveDonation(t *testing.T, userID int, email string, createdAt int64) *model.WtDonation {
	t.Helper()
	donation := &model.WtDonation{
		UserId:         userID,
		Provider:       "antigravity",
		Email:          email,
		Status:         model.WtDonationActive,
		CredentialName: "antigravity-" + email + ".json",
		AuthIndex:      "idx-" + email,
		ProjectId:      "proj-" + email,
		VerifiedAt:     createdAt,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}
	require.NoError(t, model.DB.Create(donation).Error)
	return donation
}

func wtIDParam(id int) gin.Param {
	return gin.Param{Key: "id", Value: strconv.Itoa(id)}
}

// The list carries the live state of every active donation from one cached
// credential list, hides removed ones, and leaves pool null otherwise.
func TestWtDonationListShowsThePoolState(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		fake.addCredential("fine@example.com", nil)
		fake.addCredential("cooling@example.com", map[string]any{"status": "error", "status_message": "quota exhausted", "next_retry_after": wtDonationClock.Add(20 * time.Minute).Format(time.RFC3339)})
		fine := wtActiveDonation(t, member.Id, "fine@example.com", 100)
		wtActiveDonation(t, member.Id, "cooling@example.com", 200)
		wtActiveDonation(t, member.Id, "gone@example.com", 300)
		require.NoError(t, model.DB.Create(&model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: "failed@example.com", Status: model.WtDonationFailed, FailReason: service.WtDonationFailDenied, CreatedAt: 400}).Error)
		require.NoError(t, model.DB.Create(&model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: "removed@example.com", Status: model.WtDonationRemoved, CreatedAt: 500}).Error)
		require.NoError(t, model.DB.Model(fine).Updates(map[string]any{"last_test_at": 900, "last_test_ok": 1, "last_test_latency_ms": 850}).Error)

		list := wtListDonations(t, session)
		assert.Equal(t, 10, list.Max)
		require.Len(t, list.Items, 4, "removed donations are not listed")
		emails := make([]string, 0, len(list.Items))
		for _, item := range list.Items {
			emails = append(emails, item.Email)
		}
		assert.Equal(t, []string{"failed@example.com", "gone@example.com", "cooling@example.com", "fine@example.com"}, emails, "newest first")
		assert.Equal(t, "antigravity", list.Items[0].Provider)
		assert.Nil(t, list.Items[0].Pool)
		assert.Equal(t, service.WtDonationFailDenied, list.Items[0].FailReason)
		require.NotNil(t, list.Items[1].Pool)
		assert.Equal(t, service.WtPoolMissing, list.Items[1].Pool.State)
		require.NotNil(t, list.Items[2].Pool)
		assert.Equal(t, service.WtPoolCooldown, list.Items[2].Pool.State)
		assert.Equal(t, wtDonationClock.Add(20*time.Minute).Unix(), list.Items[2].Pool.RetryAt)
		assert.Equal(t, "quota exhausted", list.Items[2].Pool.Message)
		require.NotNil(t, list.Items[3].Pool)
		assert.Equal(t, service.WtPoolOK, list.Items[3].Pool.State)
		assert.EqualValues(t, 12, list.Items[3].Pool.Success)
		assert.EqualValues(t, 1, list.Items[3].Pool.Failed)
		require.NotNil(t, list.Items[3].LastTest)
		assert.True(t, list.Items[3].LastTest.OK)
		assert.Equal(t, 850, list.Items[3].LastTest.LatencyMs)
		assert.Nil(t, list.Items[1].LastTest)

		wtListDonations(t, session)
		assert.Equal(t, 1, fake.requestsOf("GET /v8/management/credentials"), "the credential list is cached for 15 seconds")

		service.WtResetDonationCaches()
		fake.mu.Lock()
		fake.down = true
		fake.mu.Unlock()
		list = wtListDonations(t, session)
		require.NotNil(t, list.Items[3].Pool)
		assert.Equal(t, service.WtPoolUnknown, list.Items[3].Pool.State, "an unreachable pool does not fail the page")

		response := wtDonationCall(t, session, WtGetDonation, http.MethodGet, "/api/wt/donations/0", "", wtIDParam(fine.Id))
		require.True(t, response.Success, response.Message)
		var progress struct {
			ID     int    `json:"id"`
			Email  string `json:"email"`
			Status string `json:"status"`
		}
		require.NoError(t, common.Unmarshal(response.Data, &progress))
		assert.Equal(t, fine.Id, progress.ID)
		assert.Equal(t, "fine@example.com", progress.Email)
		assert.Equal(t, model.WtDonationActive, progress.Status)
	})
}

// Usage comes from Google through the pool, is parsed into buckets and a
// tier, and is served from a one-minute cache afterwards; failures are not
// cached.
func TestWtDonationUsageParsesBucketsAndTier(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		fake.addCredential("fine@example.com", nil)
		fine := wtActiveDonation(t, member.Id, "fine@example.com", 100)
		failed := &model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: "failed@example.com", Status: model.WtDonationFailed, CreatedAt: 200}
		require.NoError(t, model.DB.Create(failed).Error)
		usageOf := func(id int) securityEnrollmentResponse {
			return wtDonationCall(t, session, WtGetDonationUsage, http.MethodGet, "/api/wt/donations/0/usage", "", wtIDParam(id))
		}

		response := usageOf(fine.Id)
		require.True(t, response.Success, response.Message)
		var usage service.WtUsage
		require.NoError(t, common.Unmarshal(response.Data, &usage))
		assert.Equal(t, "pro", usage.Tier)
		assert.Equal(t, wtDonationClock.Unix(), usage.FetchedAt)
		require.Len(t, usage.Groups, 1)
		assert.Equal(t, "Gemini 3 Pro", usage.Groups[0].Name)
		require.Len(t, usage.Groups[0].Buckets, 2, "a bucket without a figure is dropped")
		assert.Equal(t, "5h", usage.Groups[0].Buckets[0].Window)
		assert.InDelta(t, 0.73, usage.Groups[0].Buckets[0].RemainingFraction, 0.001)
		assert.Equal(t, time.Date(2026, 10, 15, 7, 0, 0, 0, time.UTC).Unix(), usage.Groups[0].Buckets[0].ResetAt)
		assert.Equal(t, "weekly", usage.Groups[0].Buckets[1].Window)
		assert.InDelta(t, 0.5, usage.Groups[0].Buckets[1].RemainingFraction, 0.001)

		usageOf(fine.Id)
		assert.Equal(t, 2, fake.requestsOf("POST /v8/management/requests/api-call"), "quota and tier are asked once a minute")

		response = usageOf(failed.Id)
		assert.False(t, response.Success)
		assert.Equal(t, "Could not fetch usage for this account", response.Message)

		service.WtResetDonationCaches()
		fake.mu.Lock()
		fake.quotaBody = "not json"
		fake.mu.Unlock()
		assert.Equal(t, "Could not fetch usage for this account", usageOf(fine.Id).Message)
		fake.mu.Lock()
		fake.quotaBody = `{"groups":[]}`
		fake.mu.Unlock()
		response = usageOf(fine.Id)
		assert.True(t, response.Success, "a failure is not cached: the next call asks Google again")
	})
}

// A test asks Google afresh, records the outcome, and holds each account to
// one test a minute and twenty a Beijing day; Google failing is a failed
// test, not an error.
func TestWtDonationTestRecordsAndLimits(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		fake.addCredential("fine@example.com", nil)
		fine := wtActiveDonation(t, member.Id, "fine@example.com", 100)
		failed := &model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: "failed@example.com", Status: model.WtDonationFailed, CreatedAt: 200}
		require.NoError(t, model.DB.Create(failed).Error)
		test := func(id int) securityEnrollmentResponse {
			return wtDonationCall(t, session, WtTestDonation, http.MethodPost, "/api/wt/donations/0/test", "", wtIDParam(id))
		}
		var result struct {
			OK        bool             `json:"ok"`
			LatencyMs int              `json:"latency_ms"`
			Message   string           `json:"message"`
			Usage     *service.WtUsage `json:"usage"`
		}

		response := test(fine.Id)
		require.True(t, response.Success, response.Message)
		require.NoError(t, common.Unmarshal(response.Data, &result))
		assert.True(t, result.OK)
		assert.Empty(t, result.Message)
		require.NotNil(t, result.Usage)
		assert.Equal(t, "pro", result.Usage.Tier)
		row := wtDonationRow(t, fine.Id)
		assert.Equal(t, wtDonationClock.Unix(), row.LastTestAt)
		assert.Equal(t, 1, row.LastTestOk)
		assert.Equal(t, 1, row.TestsToday)
		assert.Equal(t, model.WtCheckinDay(wtDonationClock), row.TestsDay)

		assert.Equal(t, "Wait a minute before testing again", test(fine.Id).Message)

		pinWtClock(t, wtDonationClock.Add(2*time.Minute))
		require.NoError(t, model.DB.Model(fine).Update("tests_today", 20).Error)
		assert.Equal(t, "This account has reached today's test limit", test(fine.Id).Message)

		// A new Beijing day starts the count over.
		pinWtClock(t, wtDonationClock.Add(24*time.Hour))
		require.True(t, test(fine.Id).Success)
		assert.Equal(t, 1, wtDonationRow(t, fine.Id).TestsToday)

		pinWtClock(t, wtDonationClock.Add(24*time.Hour+2*time.Minute))
		fake.mu.Lock()
		fake.quotaBody = "not json"
		fake.mu.Unlock()
		response = test(fine.Id)
		require.True(t, response.Success, response.Message)
		require.NoError(t, common.Unmarshal(response.Data, &result))
		assert.False(t, result.OK)
		assert.Equal(t, "Unreadable usage response from Google", result.Message)
		assert.Nil(t, result.Usage)
		row = wtDonationRow(t, fine.Id)
		assert.Equal(t, 0, row.LastTestOk)
		assert.Equal(t, "Unreadable usage response from Google", row.LastTestMessage)
		assert.Equal(t, 2, row.TestsToday)

		assert.Equal(t, "Only an active donation can be tested", test(failed.Id).Message)

		var audits []model.AuditLog
		require.NoError(t, model.LOG_DB.Where("action = ?", "wt.donation_test").Order("id").Find(&audits).Error)
		require.Len(t, audits, 3, "refused tests leave no audit entry")
		assert.Contains(t, wtAuditParams(t, audits[0]), `"success":true`)
		assert.Contains(t, wtAuditParams(t, audits[0]), `"latency_ms"`)
		assert.Contains(t, wtAuditParams(t, audits[2]), `"success":false`)
		assert.NotContains(t, wtAuditParams(t, audits[0]), "example.com", "audits carry no addresses")
	})
}

// Removing deletes the credential from the pool first and closes the
// donation only when that worked; one still being verified waits ten
// minutes.
func TestWtDonationRemoveDeletesTheCredential(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		fake.addCredential("fine@example.com", nil)
		fake.addCredential("other@example.com", nil)
		fine := wtActiveDonation(t, member.Id, "fine@example.com", 100)
		other := wtActiveDonation(t, member.Id, "other@example.com", 200)
		stale := wtActiveDonation(t, member.Id, "stale@example.com", 300)
		verifying := &model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: "slow@example.com", Status: model.WtDonationVerifying, SubmittedAt: wtDonationClock.Unix(), CreatedAt: 400}
		require.NoError(t, model.DB.Create(verifying).Error)
		remove := func(id int) securityEnrollmentResponse {
			return wtDonationCall(t, session, WtRemoveDonation, http.MethodPost, "/api/wt/donations/0/remove", "", wtIDParam(id))
		}

		assert.Equal(t, "This donation is still being verified", remove(verifying.Id).Message)

		response := remove(fine.Id)
		require.True(t, response.Success, response.Message)
		assert.Equal(t, "Donation removed", response.Message)
		fake.mu.Lock()
		assert.Equal(t, []string{"antigravity-fine@example.com.json"}, fake.deleted)
		fake.mu.Unlock()
		row := wtDonationRow(t, fine.Id)
		assert.Equal(t, model.WtDonationRemoved, row.Status)
		assert.Equal(t, wtDonationClock.Unix(), row.RemovedAt)
		assert.Equal(t, "Donation does not exist", remove(fine.Id).Message, "removing twice")

		// A file the pool no longer has is not an error.
		require.True(t, remove(stale.Id).Success)
		assert.Equal(t, model.WtDonationRemoved, wtDonationRow(t, stale.Id).Status)

		// Past the grace period a stuck verification can be removed; it has
		// no credential, so the pool is not asked, and it was never in the
		// pool, so nothing is announced.
		pinWtClock(t, wtDonationClock.Add(11*time.Minute))
		deletes := fake.requestsOf("DELETE /v8/management/credentials")
		require.True(t, remove(verifying.Id).Success)
		assert.Equal(t, deletes, fake.requestsOf("DELETE /v8/management/credentials"))
		events, err := model.WtListPoolEvents(0, 10)
		require.NoError(t, err)
		require.Len(t, events, 2, "the two active accounts were announced as removed")
		assert.Equal(t, model.WtPoolEventAntigravityRemoved, events[0].Kind)
		assert.Equal(t, model.WtPoolEventAntigravityRemoved, events[1].Kind)
		assert.Equal(t, "donor", events[0].DisplayName)

		list := wtListDonations(t, session)
		require.Len(t, list.Items, 1)
		assert.Equal(t, other.Id, list.Items[0].ID)

		var audits []model.AuditLog
		require.NoError(t, model.LOG_DB.Where("action = ?", "wt.donation_remove").Order("id").Find(&audits).Error)
		require.Len(t, audits, 3)
		assert.Contains(t, wtAuditParams(t, audits[0]), `"success":true`)

		fake.mu.Lock()
		fake.down = true
		fake.mu.Unlock()
		assert.Equal(t, "The account pool is temporarily unavailable, try again later", remove(other.Id).Message)
		assert.Equal(t, model.WtDonationActive, wtDonationRow(t, other.Id).Status, "the record stays when the pool could not delete the file")
		require.NoError(t, model.LOG_DB.Where("action = ?", "wt.donation_remove").Order("id").Find(&audits).Error)
		require.Len(t, audits, 4)
		assert.Contains(t, wtAuditParams(t, audits[3]), `"success":false`)
	})
}

// Another member's donation does not exist, whichever route asks, and
// neither does a malformed id.
func TestWtDonationRoutesHideOtherMembersDonations(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		owner := createWtUser(t, model.User{Username: "donor"})
		stranger := createWtUser(t, model.User{Username: "stranger"})
		session := wtSession(t, stranger.Id)
		fake.addCredential("fine@example.com", nil)
		fine := wtActiveDonation(t, owner.Id, "fine@example.com", 100)

		routes := map[string]struct {
			method  string
			handler gin.HandlerFunc
		}{
			"get":    {http.MethodGet, WtGetDonation},
			"usage":  {http.MethodGet, WtGetDonationUsage},
			"test":   {http.MethodPost, WtTestDonation},
			"remove": {http.MethodPost, WtRemoveDonation},
		}
		for name, route := range routes {
			for _, id := range []string{strconv.Itoa(fine.Id), "0", "abc"} {
				response := wtDonationCall(t, session, route.handler, route.method, "/api/wt/donations/"+id, "", gin.Param{Key: "id", Value: id})
				assert.False(t, response.Success, name+" "+id)
				assert.Equal(t, "Donation does not exist", response.Message, name+" "+id)
			}
		}
		assert.Equal(t, model.WtDonationActive, wtDonationRow(t, fine.Id).Status)
		assert.Zero(t, fake.requestsOf("DELETE /v8/management/credentials"))
		assert.Zero(t, fake.requestsOf("POST /v8/management/requests/api-call"))
	})
}
