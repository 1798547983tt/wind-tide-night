package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wind-tide: regression tests of the AIS playground pool and the pool page
// (wind-tide/API.md §15) against a scripted stand-in for aistudio-api's
// management API; the BUILD pool has its own in wt_build_test.go.

const (
	wtTestAisPassword = "test-ais-console-password"
	wtTestAisKey      = "test-ais-relay-key"

	wtAisArgon      = "gemini-4-argon"
	wtAisFlash      = "gemini-3.8-flash"
	wtAisArgonRelay = "AIS PG-gemini-4-argon"
	wtAisFlashRelay = "AIS PG-gemini-3.8-flash"
)

// wtFakeAis plays aistudio-api for one test: it imports cookies into
// numbered accounts, answers detect-email from a script per account, keeps
// a rotation table and records the deletions it is asked for.
type wtFakeAis struct {
	t      *testing.T
	server *httptest.Server
	mu     sync.Mutex

	rejectImport bool
	down         bool
	nextAccount  int
	accounts     map[string]bool
	emails       map[string][]string // "null", "503", "404", "email" or an address; the last one repeats
	rotation     map[string]map[string]any
	deleted      []string
	requests     []string
	imports      []map[string]string
}

func newWtFakeAis(t *testing.T) *wtFakeAis {
	t.Helper()
	fake := &wtFakeAis{t: t, accounts: map[string]bool{}, emails: map[string][]string{}, rotation: map[string]map[string]any{}}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	t.Setenv("WT_AIS_URL", fake.server.URL)
	t.Setenv("WT_AIS_PASSWORD", wtTestAisPassword)
	t.Setenv("WT_AIS_API_KEY", wtTestAisKey)
	interval, inline := service.WtAisVerifyInterval, service.WtDonationVerifySync
	service.WtAisVerifyInterval, service.WtDonationVerifySync = time.Millisecond, true
	t.Cleanup(func() {
		service.WtAisVerifyInterval, service.WtDonationVerifySync = interval, inline
	})
	service.WtResetDonationCaches()
	t.Cleanup(service.WtResetDonationCaches)
	return fake
}

func (fake *wtFakeAis) handle(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	route := r.Method + " " + r.URL.Path
	fake.requests = append(fake.requests, route)
	if fake.down {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+wtTestAisPassword {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	assert.Equal(fake.t, "application/json", r.Header.Get("Accept"))
	id, action, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/accounts/"), "/")
	switch {
	case route == "POST /accounts/import-cookies":
		var body map[string]string
		assert.NoError(fake.t, common.DecodeJson(r.Body, &body))
		fake.imports = append(fake.imports, body)
		if fake.rejectImport {
			wtJSONReply(w, http.StatusBadRequest, map[string]any{"detail": "未解析到有效 cookie"})
			return
		}
		fake.nextAccount++
		account := fmt.Sprintf("ais-acc-%d", fake.nextAccount)
		fake.accounts[account] = true
		wtJSONReply(w, http.StatusOK, map[string]any{"account_id": account, "name": body["name"], "cookie_count": 3, "domain_summary": map[string]int{"google.com": 3}, "auth_user": body["auth_user"]})
	case route == "GET /rotation/accounts":
		wtJSONReply(w, http.StatusOK, fake.rotation)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/accounts/") && action == "":
		fake.deleted = append(fake.deleted, id)
		if !fake.accounts[id] {
			wtJSONReply(w, http.StatusNotFound, map[string]any{"detail": "账号不存在"})
			return
		}
		delete(fake.accounts, id)
		wtJSONReply(w, http.StatusOK, map[string]any{"ok": true})
	case r.Method == http.MethodPost && action == "detect-email":
		if !fake.accounts[id] {
			wtJSONReply(w, http.StatusNotFound, map[string]any{"detail": "账号不存在"})
			return
		}
		answer := "email"
		if script := fake.emails[id]; len(script) > 0 {
			answer = script[0]
			if len(script) > 1 {
				fake.emails[id] = script[1:]
			}
		}
		switch answer {
		case "null":
			wtJSONReply(w, http.StatusOK, map[string]any{"id": id, "name": "wt", "email": nil, "auth_user": "0"})
		case "503":
			wtJSONReply(w, http.StatusServiceUnavailable, map[string]any{"detail": "受控浏览器未就绪"})
		case "404":
			wtJSONReply(w, http.StatusNotFound, map[string]any{"detail": "账号不存在"})
		case "email":
			wtJSONReply(w, http.StatusOK, map[string]any{"id": id, "name": "wt", "email": id + "@example.com", "auth_user": "0"})
		default:
			wtJSONReply(w, http.StatusOK, map[string]any{"id": id, "name": "wt", "email": answer, "auth_user": "0"})
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func wtJSONReply(w http.ResponseWriter, status int, body any) {
	encoded, err := common.Marshal(body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

// nextAccountID is the id the next import gets.
func (fake *wtFakeAis) nextAccountID() string {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fmt.Sprintf("ais-acc-%d", fake.nextAccount+1)
}

// script sets the detect-email answers of an account.
func (fake *wtFakeAis) script(account string, answers ...string) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.emails[account] = answers
}

// hold puts an account in the pool without an import.
func (fake *wtFakeAis) hold(account string) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.accounts[account] = true
}

// requestsOf counts the calls of one route, "METHOD /path".
func (fake *wtFakeAis) requestsOf(route string) int {
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

// wtAisCall runs a handler for a signed-in member and returns the HTTP
// status with the decoded body.
func wtAisCall(t *testing.T, member service.AuthIdentity, handler gin.HandlerFunc, method, target, body string, params ...gin.Param) (int, securityEnrollmentResponse) {
	t.Helper()
	recorder := wtRequest{method: method, target: target, body: body, identity: member, params: params}.serve(handler)
	return recorder.Code, decodeSecurityEnrollmentResponse(t, recorder)
}

// wtAisDonate pastes a Cookie for a member.
func wtAisDonate(t *testing.T, member service.AuthIdentity, cookies string) (int, securityEnrollmentResponse) {
	t.Helper()
	body, err := common.Marshal(map[string]any{"cookies": cookies, "accepted": true})
	require.NoError(t, err)
	return wtAisCall(t, member, WtDonateAistudio, http.MethodPost, "/api/wt/donations/aistudio", string(body))
}

// wtAisDonation seeds an active AI Studio donation bound to a pool account.
func wtAisDonation(t *testing.T, userID int, account, email string, createdAt int64) *model.WtDonation {
	t.Helper()
	donation := &model.WtDonation{
		UserId:         userID,
		Provider:       model.WtAisProvider,
		Email:          email,
		Status:         model.WtDonationActive,
		CredentialName: account,
		AuthIndex:      "fp-" + account,
		VerifiedAt:     createdAt,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}
	require.NoError(t, model.DB.Create(donation).Error)
	return donation
}

// wtPoolEvents is the feed, newest first.
func wtPoolEvents(t *testing.T) []*model.WtPoolEvent {
	t.Helper()
	events, err := model.WtListPoolEvents(0, 50)
	require.NoError(t, err)
	return events
}

func wtFingerprintOf(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])[:32]
}

// wtAisMoveBody is the body of a give or take.
func wtAisMoveBody(upstream string, count int) string {
	return fmt.Sprintf(`{"model":%q,"count":%d}`, upstream, count)
}

// wtAisModelOf picks one model's balance out of the pool page.
func wtAisModelOf(t *testing.T, balance *model.WtAisBalance, upstream string) model.WtAisModelBalance {
	t.Helper()
	require.NotNil(t, balance)
	for _, entry := range balance.Models {
		if entry.Model == upstream {
			return entry
		}
	}
	require.Failf(t, "model missing from the pool page", "%s", upstream)
	return model.WtAisModelBalance{}
}

// The model table of the pools: two AIS PG relay models keyed by their
// upstream names and one BUILD relay model keyed by its own name, found by
// relay name or by ledger key, exactly.
func TestWtAisModelTable(t *testing.T) {
	require.Len(t, model.WtAisModels, 3)
	argon, ok := model.WtAisModelByName(wtAisArgonRelay)
	require.True(t, ok)
	assert.Equal(t, model.WtAisModelSpec{Provider: model.WtAisProvider, Name: wtAisArgonRelay, Upstream: wtAisArgon, Key: wtAisArgon, CallsPerAccount: 50, TakeMaxPerDay: 100}, *argon)
	flash, ok := model.WtAisModelByKey(wtAisFlash)
	require.True(t, ok)
	assert.Equal(t, model.WtAisModelSpec{Provider: model.WtAisProvider, Name: wtAisFlashRelay, Upstream: wtAisFlash, Key: wtAisFlash, CallsPerAccount: 100, TakeMaxPerDay: 100}, *flash)
	build, ok := model.WtAisModelByKey(wtBuildFlashRelay)
	require.True(t, ok)
	assert.Equal(t, model.WtAisModelSpec{Provider: model.WtBuildProvider, Name: wtBuildFlashRelay, Upstream: wtAisFlash, Key: wtBuildFlashRelay, CallsPerAccount: 100, TakeMaxPerDay: 100}, *build, "the BUILD ledger has its own key, as its upstream is the AIS flash model")
	aisSpecs := model.WtAisModelsOf(model.WtAisProvider)
	require.Len(t, aisSpecs, 2)
	for _, spec := range aisSpecs {
		assert.Equal(t, service.WtAisModelPrefix+spec.Upstream, spec.Name, "the relay name is the prefixed upstream name")
	}
	buildSpecs := model.WtAisModelsOf(model.WtBuildProvider)
	require.Len(t, buildSpecs, 1)
	assert.Equal(t, service.WtBuildModelPrefix+wtAisFlash, buildSpecs[0].Name)
	_, ok = model.WtAisModelByName(wtAisArgon)
	assert.False(t, ok, "ledger keys are not relay names")
	_, ok = model.WtAisModelByKey(wtAisArgonRelay)
	assert.False(t, ok, "AIS PG relay names are not ledger keys")
	_, ok = model.WtAisModelByName(wtAisArgonRelay + " ")
	assert.False(t, ok, "exact match")
}

// Pasted cookie text is accepted as a header string, a JSON array or the
// Android extractor bundle; the fingerprint comes from the first sign-in
// cookie present and is the same whichever form carried it.
func TestWtAisCookieParsing(t *testing.T) {
	cases := []struct {
		name, raw, header, fingerprint string
	}{
		{"header", "__Secure-1PSID=abc; SID=def; HSID=x", "__Secure-1PSID=abc; SID=def; HSID=x", wtFingerprintOf("abc")},
		{"header with stray spaces", " SID=def ;SAPISID=ghi;\n", "SID=def; SAPISID=ghi", wtFingerprintOf("def")},
		{"SAPISID only", "HSID=x; SAPISID=ghi", "HSID=x; SAPISID=ghi", wtFingerprintOf("ghi")},
		{"1PAPISID only", "__Secure-1PAPISID=jkl", "__Secure-1PAPISID=jkl", wtFingerprintOf("jkl")},
		{"json array", `[{"name":"HSID","value":"x","domain":".google.com"},{"name":"__Secure-1PSID","value":"abc"}]`, "HSID=x; __Secure-1PSID=abc", wtFingerprintOf("abc")},
		{"android bundle", `{"accounts":[{"email":"a@b.c","cookies":"SID=def; HSID=x"},{"cookies":"SID=other"}]}`, "SID=def; HSID=x", wtFingerprintOf("def")},
		{"android bundle with array cookies", `{"profiles":[{"cookies":[{"name":"SID","value":"def"}]}]}`, "SID=def", wtFingerprintOf("def")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			header, fingerprint, ok := service.WtAisParseCookies(tc.raw)
			require.True(t, ok)
			assert.Equal(t, tc.header, header)
			assert.Equal(t, tc.fingerprint, fingerprint)
			assert.Len(t, fingerprint, 32)
		})
	}
	for _, raw := range []string{"", "HSID=x; APISID=y", "not cookies", "{", "[1,2]", `{"accounts":[]}`, "__Secure-1PSID=", `[{"name":"SID"}]`} {
		_, _, ok := service.WtAisParseCookies(raw)
		assert.False(t, ok, raw)
	}
}

// Without the three AIS variables the pool is off: /api/status says so, the
// AIS routes answer the not-configured message and the pool page needs at
// least one pool.
func TestWtAisDisabledWithoutPool(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		for _, name := range []string{"WT_CPA_URL", "WT_CPA_MANAGEMENT_KEY", "WT_AIS_URL", "WT_AIS_PASSWORD", "WT_AIS_API_KEY", "WT_BUILD_URL", "WT_BUILD_PASSWORD", "WT_BUILD_API_KEY"} {
			t.Setenv(name, "")
		}
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)

		status := decodeSecurityEnrollmentResponse(t, wtServe(GetStatus, http.MethodGet, "/api/status", "", 0))
		var data map[string]any
		require.NoError(t, common.Unmarshal(status.Data, &data))
		assert.Equal(t, false, data["wt_ais_enabled"])
		assert.Equal(t, false, data["wt_build_enabled"])

		const notConfigured = "The AIS playground pool is not configured"
		_, response := wtAisDonate(t, session, "SID=abc")
		assert.Equal(t, notConfigured, response.Message)
		_, response = wtAisCall(t, session, WtGiveAisCalls, http.MethodPost, "/api/wt/pool/ais/give", wtAisMoveBody(wtAisArgon, 1))
		assert.Equal(t, notConfigured, response.Message)
		_, response = wtAisCall(t, session, WtListDonations, http.MethodGet, "/api/wt/donations?provider=aistudio", "")
		assert.Equal(t, notConfigured, response.Message)
		_, response = wtAisCall(t, session, WtGetPool, http.MethodGet, "/api/wt/pool", "")
		assert.Equal(t, "Account donation is not enabled on this site", response.Message)

		t.Setenv("WT_AIS_URL", "http://aistudio-api.test")
		t.Setenv("WT_AIS_PASSWORD", "x")
		assert.False(t, service.WtAisEnabled(), "all three variables are needed")
		t.Setenv("WT_AIS_API_KEY", "y")
		assert.True(t, service.WtAisEnabled())
		status = decodeSecurityEnrollmentResponse(t, wtServe(GetStatus, http.MethodGet, "/api/status", "", 0))
		require.NoError(t, common.Unmarshal(status.Data, &data))
		assert.Equal(t, true, data["wt_ais_enabled"])

		// The BUILD pool stays off on its own: its model, its donations and
		// its list answer the BUILD message while the AIS pool works.
		const buildNotConfigured = "The BUILD pool is not configured"
		_, response = wtAisCall(t, session, WtGiveAisCalls, http.MethodPost, "/api/wt/pool/ais/give", wtAisMoveBody(wtBuildFlashRelay, 1))
		assert.Equal(t, buildNotConfigured, response.Message)
		_, response = wtBuildDonate(t, session, "SID=abc")
		assert.Equal(t, buildNotConfigured, response.Message)
		_, response = wtAisCall(t, session, WtListDonations, http.MethodGet, "/api/wt/donations?provider=build", "")
		assert.Equal(t, buildNotConfigured, response.Message)

		// The pool page works with the AI Studio pool alone: one balance per
		// model, in table order, empty for a member without accounts.
		pool := wtReadPool(t, session)
		assert.Nil(t, pool.Antigravity)
		assert.Nil(t, pool.Build)
		require.NotNil(t, pool.Aistudio)
		assert.Equal(t, model.WtAisProvider, pool.Aistudio.Provider)
		assert.Equal(t, model.WtCheckinDay(model.WtNow()), pool.Aistudio.Day, "the day is a check-in day")
		assert.Zero(t, pool.Aistudio.Accounts)
		require.Len(t, pool.Aistudio.Models, 2)
		assert.Equal(t, model.WtAisModelBalance{Provider: model.WtAisProvider, Model: wtAisArgon, RelayModel: wtAisArgonRelay, PerAccount: 50, TakeMax: 100, TakeLeft: 100}, pool.Aistudio.Models[0])
		assert.Equal(t, model.WtAisModelBalance{Provider: model.WtAisProvider, Model: wtAisFlash, RelayModel: wtAisFlashRelay, PerAccount: 100, TakeMax: 100, TakeLeft: 100}, pool.Aistudio.Models[1])
		assert.NotNil(t, pool.Events)
	})
}

// Donating an AI Studio account: the consent and the Cookie are checked
// before the pool is asked, the import answer becomes a verifying donation,
// and the verification binds the email the pool reads, or fails and takes
// the account out of the pool again.
func TestWtAisDonateFlow(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeAis(t)
		member := createWtUser(t, model.User{Username: "donor", DisplayName: "Dora"})
		other := createWtUser(t, model.User{Username: "other"})
		session := wtSession(t, member.Id)
		const cookieError = "Paste the Cookie of a signed-in AI Studio page"

		status, response := wtAisCall(t, session, WtDonateAistudio, http.MethodPost, "/api/wt/donations/aistudio", `{"cookies":"SID=abc"}`)
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, "Accept the risks before donating an account", response.Message)
		status, response = wtAisCall(t, session, WtDonateAistudio, http.MethodPost, "/api/wt/donations/aistudio", `{"cookies":`)
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, "Invalid parameters", response.Message)
		for _, bad := range []string{"HSID=x; APISID=y", "", "SID=" + strings.Repeat("a", service.WtAisCookieMaxBytes)} {
			status, response = wtAisDonate(t, session, bad)
			assert.Equal(t, http.StatusBadRequest, status)
			assert.Equal(t, cookieError, response.Message)
		}
		assert.Zero(t, fake.requestsOf("POST /accounts/import-cookies"), "nothing reached the pool")

		fake.mu.Lock()
		fake.rejectImport = true
		fake.mu.Unlock()
		status, response = wtAisDonate(t, session, "SID=rejected")
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, cookieError, response.Message)
		fake.mu.Lock()
		fake.rejectImport, fake.down = false, true
		fake.mu.Unlock()
		status, response = wtAisDonate(t, session, "SID=down")
		assert.Equal(t, http.StatusBadGateway, status)
		assert.Equal(t, "The pool is not answering; try again later", response.Message)
		fake.mu.Lock()
		fake.down = false
		fake.mu.Unlock()
		var count int64
		require.NoError(t, model.DB.Model(&model.WtDonation{}).Count(&count).Error)
		assert.Zero(t, count, "a refused import leaves no row")

		// Success: the Cookie is forwarded once as a header string and the
		// verification binds the email the pool read.
		status, response = wtAisDonate(t, session, `[{"name":"__Secure-1PSID","value":"secret-1"},{"name":"HSID","value":"h"}]`)
		require.Equal(t, http.StatusAccepted, status, response.Message)
		require.True(t, response.Success)
		var accepted struct {
			ID     int    `json:"id"`
			Status string `json:"status"`
		}
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		assert.Equal(t, model.WtDonationVerifying, accepted.Status)
		row := wtDonationRow(t, accepted.ID)
		assert.Equal(t, model.WtAisProvider, row.Provider)
		assert.Equal(t, model.WtDonationActive, row.Status)
		assert.Equal(t, "ais-acc-1@example.com", row.Email)
		assert.Equal(t, "ais-acc-1", row.CredentialName)
		assert.Equal(t, wtFingerprintOf("secret-1"), row.AuthIndex)
		assert.Empty(t, row.ProjectId)
		assert.Equal(t, wtDonationClock.Unix(), row.VerifiedAt)
		fake.mu.Lock()
		require.Len(t, fake.imports, 2, "the pool that was down saw no import")
		assert.Equal(t, "__Secure-1PSID=secret-1; HSID=h", fake.imports[1]["cookies"])
		assert.Regexp(t, regexp.MustCompile(`^wt-`+strconv.Itoa(member.Id)+`-[0-9a-f]{4}$`), fake.imports[1]["name"])
		assert.Equal(t, "0", fake.imports[1]["auth_user"])
		fake.mu.Unlock()
		events := wtPoolEvents(t)
		require.Len(t, events, 1)
		assert.Equal(t, model.WtPoolEventAisDonated, events[0].Kind)
		assert.Equal(t, model.WtAisProvider, events[0].Provider)
		assert.Equal(t, "Dora", events[0].DisplayName)
		assert.Equal(t, 1, events[0].Amount)
		assert.Empty(t, events[0].Model, "account events carry no model")

		// The same account again, in header form, is refused before the pool
		// is asked.
		status, response = wtAisDonate(t, session, "__Secure-1PSID=secret-1; SID=something-else")
		assert.Equal(t, http.StatusOK, status)
		assert.Equal(t, "This account is already in the pool", response.Message)
		assert.Equal(t, 3, fake.requestsOf("POST /accounts/import-cookies"))

		// A page the pool could not read yet is retried.
		fake.script(fake.nextAccountID(), "null", "503", "email")
		_, response = wtAisDonate(t, session, "SID=secret-2")
		require.True(t, response.Success, response.Message)
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		assert.Equal(t, model.WtDonationActive, wtDonationRow(t, accepted.ID).Status)
		assert.Equal(t, 3, fake.requestsOf("POST /accounts/ais-acc-2/detect-email"))

		// An email another member already donated fails the donation and
		// deletes the account from the pool.
		wtAisDonation(t, other.Id, "ais-acc-other", "taken@example.com", 100)
		fake.script(fake.nextAccountID(), "Taken@Example.com")
		_, response = wtAisDonate(t, session, "SID=secret-3")
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		row = wtDonationRow(t, accepted.ID)
		assert.Equal(t, model.WtDonationFailed, row.Status)
		assert.Equal(t, "This account is already in the pool", row.FailReason)
		assert.Empty(t, row.Email)
		fake.mu.Lock()
		assert.Equal(t, []string{"ais-acc-3"}, fake.deleted)
		fake.mu.Unlock()

		// An account the pool lost fails at once.
		fake.script(fake.nextAccountID(), "404")
		_, response = wtAisDonate(t, session, "SID=secret-4")
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		row = wtDonationRow(t, accepted.ID)
		assert.Equal(t, model.WtDonationFailed, row.Status)
		assert.Equal(t, "The pool lost the account; donate again", row.FailReason)
		fake.mu.Lock()
		assert.Contains(t, fake.deleted, "ais-acc-4")
		fake.mu.Unlock()

		// A page that never becomes readable fails after the last attempt.
		fake.script(fake.nextAccountID(), "null")
		_, response = wtAisDonate(t, session, "SID=secret-5")
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		row = wtDonationRow(t, accepted.ID)
		assert.Equal(t, model.WtDonationFailed, row.Status)
		assert.Equal(t, "The pool could not read the account in time; try again later", row.FailReason)
		assert.Equal(t, service.WtAisVerifyAttempts, fake.requestsOf("POST /accounts/ais-acc-5/detect-email"))
		assert.Len(t, wtPoolEvents(t), 2, "failed donations are not announced")

		// Ten verifying or active AI Studio donations fill the slots; failed
		// ones and Antigravity ones do not count.
		for i := range 8 {
			require.NoError(t, model.DB.Create(&model.WtDonation{UserId: member.Id, Provider: model.WtAisProvider, Status: model.WtDonationVerifying, CredentialName: fmt.Sprintf("slot-%d", i), AuthIndex: fmt.Sprintf("slot-fp-%d", i)}).Error)
		}
		require.NoError(t, model.DB.Create(&model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: "ag@example.com", Status: model.WtDonationActive}).Error)
		imports := fake.requestsOf("POST /accounts/import-cookies")
		status, response = wtAisDonate(t, session, "SID=secret-6")
		assert.Equal(t, http.StatusOK, status)
		assert.Equal(t, "You can donate at most 10 accounts", response.Message)
		assert.Equal(t, "You can donate at most {{max}} accounts", response.MessageKey)
		assert.Equal(t, imports, fake.requestsOf("POST /accounts/import-cookies"))

		audits := wtAudits(t, "wt.donation_add")
		require.NotEmpty(t, audits)
		for _, audit := range audits {
			params := wtAuditParams(t, audit)
			assert.Contains(t, params, `"provider":"aistudio"`)
			assert.NotContains(t, params, "secret", "audits carry no cookie values")
		}
		assert.Contains(t, wtAuditParams(t, audits[0]), `"success":false`)
		assert.Contains(t, wtAuditParams(t, audits[2]), `"success":true`)
	})
}

// The list is filtered by provider and carries the pool state of every
// active AI Studio account from one cached rotation table: today's requests
// of both models added up, the models whose quota is used up listed, and the
// account exhausted only when every model is.
func TestWtAisListShowsThePoolState(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		cpa := newWtFakeCPA(t)
		fake := newWtFakeAis(t)
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		cpa.addCredential("fine@example.com", nil)
		wtActiveDonation(t, member.Id, "fine@example.com", 100)
		wtAisDonation(t, member.Id, "acc-missing", "missing@example.com", 200)
		wtAisDonation(t, member.Id, "acc-expired", "expired@example.com", 300)
		wtAisDonation(t, member.Id, "acc-cooldown", "cooldown@example.com", 400)
		wtAisDonation(t, member.Id, "acc-exhausted", "exhausted@example.com", 500)
		wtAisDonation(t, member.Id, "acc-available", "available@example.com", 600)
		wtAisDonation(t, member.Id, "acc-stale", "stale@example.com", 700)
		wtAisDonation(t, member.Id, "acc-halfcool", "halfcool@example.com", 750)
		require.NoError(t, model.DB.Create(&model.WtDonation{UserId: member.Id, Provider: model.WtAisProvider, Status: model.WtDonationFailed, FailReason: service.WtAisFailGone, CredentialName: "acc-failed", CreatedAt: 800}).Error)
		now := float64(wtDonationClock.Unix())
		// 02:00 UTC is the previous day in Los Angeles, summer or winter.
		const pacificToday, pacificYesterday = "2026-10-14", "2026-10-13"
		fake.mu.Lock()
		fake.rotation = map[string]map[string]any{
			"acc-expired":   {"account_id": "acc-expired", "session_expired": true, "auth_cooldown": now + 600, "success": 3},
			"acc-cooldown":  {"account_id": "acc-cooldown", "auth_cooldown": now + 600, "model_cooldowns": map[string]any{wtAisArgon: now + 1800}},
			"acc-exhausted": {"account_id": "acc-exhausted", "auth_cooldown": now - 60, "model_cooldowns": map[string]any{wtAisArgon: now + 1800, wtAisFlash: now + 900, "other": now + 9000}},
			"acc-available": {"account_id": "acc-available", "requests": 10, "success": 8, "errors": 1, "auth_errors": 1, "rate_limited": 2, "last_used": now, "auth_cooldown": 0, "model_cooldowns": map[string]any{wtAisArgon: now - 1}, "model_requests": map[string]any{wtAisArgon: 7, wtAisFlash: 5, "other": 9}, "model_requests_dates": map[string]any{wtAisArgon: pacificToday, wtAisFlash: pacificToday, "other": pacificToday}},
			"acc-stale":     {"account_id": "acc-stale", "model_requests": map[string]any{wtAisArgon: 7, wtAisFlash: 4}, "model_requests_dates": map[string]any{wtAisArgon: pacificYesterday, wtAisFlash: pacificToday}},
			"acc-halfcool":  {"account_id": "acc-halfcool", "model_cooldowns": map[string]any{wtAisFlash: now + 300}},
		}
		fake.mu.Unlock()

		antigravity := wtListDonations(t, session)
		require.Len(t, antigravity.Items, 1, "the default list is the Antigravity pool")
		assert.Equal(t, "fine@example.com", antigravity.Items[0].Email)

		type aisItem struct {
			ID         int                    `json:"id"`
			Provider   string                 `json:"provider"`
			Email      string                 `json:"email"`
			Status     string                 `json:"status"`
			FailReason string                 `json:"fail_reason"`
			Pool       *service.WtAisPoolInfo `json:"pool"`
		}
		var list struct {
			Max   int       `json:"max"`
			Items []aisItem `json:"items"`
		}
		listAis := func() {
			_, response := wtAisCall(t, session, WtListDonations, http.MethodGet, "/api/wt/donations?provider=aistudio", "")
			require.True(t, response.Success, response.Message)
			require.NoError(t, common.Unmarshal(response.Data, &list))
		}
		listAis()
		assert.Equal(t, 10, list.Max)
		require.Len(t, list.Items, 8)
		byEmail := map[string]aisItem{}
		for _, item := range list.Items {
			assert.Equal(t, model.WtAisProvider, item.Provider)
			byEmail[item.Email] = item
		}
		assert.Equal(t, "", list.Items[0].Email, "newest first: the failed row")
		assert.Equal(t, service.WtAisFailGone, list.Items[0].FailReason)
		assert.Nil(t, list.Items[0].Pool)
		none := []service.WtAisCooling{}
		for email, want := range map[string]service.WtAisPoolInfo{
			"missing@example.com":   {State: service.WtAisStateMissing, Message: "The pool no longer has this account", Cooling: none},
			"expired@example.com":   {State: service.WtAisStateExpired, Message: "The pool can no longer sign in with this account; remove it and donate again", Success: 3, Cooling: none},
			"cooldown@example.com":  {State: service.WtAisStateCooldown, Message: "The pool paused this account after a sign-in error", RetryAt: int64(now) + 600, Cooling: []service.WtAisCooling{{Model: wtAisArgon, RetryAt: int64(now) + 1800}}},
			"exhausted@example.com": {State: service.WtAisStateExhausted, Message: "Today's upstream quota of this account is used up", RetryAt: int64(now) + 900, Cooling: []service.WtAisCooling{{Model: wtAisArgon, RetryAt: int64(now) + 1800}, {Model: wtAisFlash, RetryAt: int64(now) + 900}}},
			"available@example.com": {State: service.WtAisStateAvailable, Message: "Available", Success: 8, Failed: 2, RateLimited: 2, RequestsToday: 12, Cooling: none},
			"stale@example.com":     {State: service.WtAisStateAvailable, Message: "Available", RequestsToday: 4, Cooling: none},
			"halfcool@example.com":  {State: service.WtAisStateAvailable, Message: "Available", Cooling: []service.WtAisCooling{{Model: wtAisFlash, RetryAt: int64(now) + 300}}},
		} {
			require.NotNil(t, byEmail[email].Pool, email)
			assert.Equal(t, want, *byEmail[email].Pool, email)
		}

		listAis()
		assert.Equal(t, 1, fake.requestsOf("GET /rotation/accounts"), "the rotation table is cached for 15 seconds")

		service.WtResetDonationCaches()
		fake.mu.Lock()
		fake.down = true
		fake.mu.Unlock()
		listAis()
		require.Len(t, list.Items, 8, "an unreachable pool does not fail the page")
		for _, item := range list.Items {
			assert.Nil(t, item.Pool, "no state is shown while the pool is down")
		}

		_, response := wtAisCall(t, session, WtListDonations, http.MethodGet, "/api/wt/donations?provider=bogus", "")
		assert.Equal(t, "Invalid parameters", response.Message)
	})
}

// Testing an AI Studio account asks the pool for its email; usage does not
// exist for this pool; removing deletes the pool account first and
// announces the removal of an active one.
func TestWtAisTestUsageAndRemove(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeAis(t)
		t.Setenv("WT_CPA_URL", "")
		t.Setenv("WT_CPA_MANAGEMENT_KEY", "")
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		fake.hold("acc-1")
		fake.hold("acc-3")
		fine := wtAisDonation(t, member.Id, "acc-1", "fine@example.com", 100)
		failed := &model.WtDonation{UserId: member.Id, Provider: model.WtAisProvider, Status: model.WtDonationFailed, CredentialName: "acc-2", CreatedAt: 200}
		require.NoError(t, model.DB.Create(failed).Error)
		stuck := wtAisDonation(t, member.Id, "acc-3", "stuck@example.com", 300)
		var result struct {
			OK        bool             `json:"ok"`
			LatencyMs int              `json:"latency_ms"`
			Message   string           `json:"message"`
			Usage     *service.WtUsage `json:"usage"`
		}
		test := func(id int) securityEnrollmentResponse {
			_, response := wtAisCall(t, session, WtTestDonation, http.MethodPost, "/api/wt/donations/0/test", "", wtIDParam(id))
			return response
		}

		response := test(fine.Id)
		require.True(t, response.Success, response.Message)
		require.NoError(t, common.Unmarshal(response.Data, &result))
		assert.True(t, result.OK)
		assert.Equal(t, "The pool can still use this account", result.Message)
		assert.Nil(t, result.Usage)
		row := wtDonationRow(t, fine.Id)
		assert.Equal(t, 1, row.LastTestOk)
		assert.Equal(t, 1, row.TestsToday)
		assert.Equal(t, "The pool can still use this account", row.LastTestMessage)
		assert.Equal(t, "Wait a minute before testing again", test(fine.Id).Message)

		pinWtClock(t, wtDonationClock.Add(2*time.Minute))
		fake.script("acc-1", "null")
		response = test(fine.Id)
		require.True(t, response.Success, response.Message)
		require.NoError(t, common.Unmarshal(response.Data, &result))
		assert.False(t, result.OK)
		assert.Equal(t, "The pool can no longer sign in with this account", result.Message)
		assert.Equal(t, 0, wtDonationRow(t, fine.Id).LastTestOk)
		assert.Equal(t, "Only an active donation can be tested", test(failed.Id).Message)
		audits := wtAudits(t, "wt.donation_test")
		require.Len(t, audits, 2)
		assert.Contains(t, wtAuditParams(t, audits[0]), `"provider":"aistudio"`)

		status, response := wtAisCall(t, session, WtGetDonationUsage, http.MethodGet, "/api/wt/donations/0/usage", "", wtIDParam(fine.Id))
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, "Usage is not available for this pool", response.Message)

		_, response = wtAisCall(t, session, WtGetDonation, http.MethodGet, "/api/wt/donations/0", "", wtIDParam(fine.Id))
		require.True(t, response.Success, response.Message)
		var progress struct {
			Provider string `json:"provider"`
			Status   string `json:"status"`
		}
		require.NoError(t, common.Unmarshal(response.Data, &progress))
		assert.Equal(t, model.WtAisProvider, progress.Provider)
		assert.Equal(t, model.WtDonationActive, progress.Status)

		remove := func(id int) securityEnrollmentResponse {
			_, response := wtAisCall(t, session, WtRemoveDonation, http.MethodPost, "/api/wt/donations/0/remove", "", wtIDParam(id))
			return response
		}
		response = remove(fine.Id)
		require.True(t, response.Success, response.Message)
		assert.Equal(t, "Donation removed", response.Message)
		fake.mu.Lock()
		assert.Equal(t, []string{"acc-1"}, fake.deleted)
		fake.mu.Unlock()
		row = wtDonationRow(t, fine.Id)
		assert.Equal(t, model.WtDonationRemoved, row.Status)
		assert.Equal(t, wtDonationClock.Add(2*time.Minute).Unix(), row.RemovedAt)
		events := wtPoolEvents(t)
		require.Len(t, events, 1)
		assert.Equal(t, model.WtPoolEventAisRemoved, events[0].Kind)
		assert.Equal(t, model.WtAisProvider, events[0].Provider)
		assert.Equal(t, "donor", events[0].DisplayName)
		assert.Empty(t, events[0].Model)

		// An account the pool no longer has is not an error, and a failed
		// donation was never announced.
		require.True(t, remove(failed.Id).Success)
		assert.Equal(t, model.WtDonationRemoved, wtDonationRow(t, failed.Id).Status)
		assert.Len(t, wtPoolEvents(t), 1)

		fake.mu.Lock()
		fake.down = true
		fake.mu.Unlock()
		assert.Equal(t, "The pool is not answering; try again later", remove(stuck.Id).Message)
		assert.Equal(t, model.WtDonationActive, wtDonationRow(t, stuck.Id).Status, "the record stays when the pool could not delete the account")
		audits = wtAudits(t, "wt.donation_remove")
		require.Len(t, audits, 3)
		assert.Contains(t, wtAuditParams(t, audits[2]), `"success":false`)
	})
}

// wtAisRelayCall posts a chat completion with an API key through a relay
// chain that ends in the gate.
func wtAisRelayCall(relay *gin.Engine, key string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
	request.Header.Set("Authorization", "Bearer sk-"+key)
	relay.ServeHTTP(recorder, request)
	return recorder
}

// The gate charges one call of an "AIS PG-" model per request against that
// model's ledger: non-donors are refused, donors get the model's share per
// active account a day, a failed upstream request is refunded and counted,
// the owner is exempt, an unknown AIS PG model is refused and other models
// are not touched. Every finished call moves the member's day counters and
// streaks behind the leaderboards.
func TestWtPoolGate(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		for _, name := range []string{"WT_AIS_URL", "WT_AIS_PASSWORD", "WT_AIS_API_KEY", "WT_BUILD_URL", "WT_BUILD_PASSWORD", "WT_BUILD_API_KEY"} {
			t.Setenv(name, "")
		}
		var modelName atomic.Value
		modelName.Store(wtAisArgonRelay)
		var upstreamFails atomic.Bool
		relay := gin.New()
		relay.POST("/v1/chat/completions", middleware.TokenAuth(), func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName.Load().(string))
		}, middleware.WtPoolGate(), func(c *gin.Context) {
			if upstreamFails.Load() {
				c.JSON(http.StatusBadGateway, gin.H{"error": "upstream"})
				return
			}
			c.Status(http.StatusNoContent)
		})
		require.NoError(t, model.WtBootstrapOwner())
		owner, err := model.WtOwnerAccount()
		require.NoError(t, err)
		donor := createWtUser(t, model.User{Username: "donor"})
		stranger := createWtUser(t, model.User{Username: "stranger"})
		donorKey := createWtAPIKey(t, donor.Id, common.TokenStatusEnabled)
		strangerKey := createWtAPIKey(t, stranger.Id, common.TokenStatusEnabled)
		ownerKey := createWtAPIKey(t, owner.Id, common.TokenStatusEnabled)
		wtAisDonation(t, donor.Id, "acc-1", "fine@example.com", 100)
		ledgerOf := func(userID int, upstream string) model.WtAisLedger {
			var ledger model.WtAisLedger
			require.NoError(t, model.DB.Where("user_id = ? AND day = ? AND model = ?", userID, model.WtCheckinDay(model.WtNow()), upstream).Limit(1).Find(&ledger).Error)
			return ledger
		}

		recorder := wtAisRelayCall(relay, donorKey)
		assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "The AIS playground pool is not configured")
		modelName.Store(wtBuildFlashRelay)
		recorder = wtAisRelayCall(relay, donorKey)
		assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "The BUILD pool is not configured")
		modelName.Store(wtAisArgonRelay)

		newWtFakeAis(t)
		modelName.Store("AIS PG-gemini-9")
		recorder = wtAisRelayCall(relay, donorKey)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "Unknown AIS PG model")
		assert.Zero(t, ledgerOf(donor.Id, "gemini-9").Id, "an unknown model gets no ledger row")
		modelName.Store(wtAisArgonRelay)

		recorder = wtAisRelayCall(relay, strangerKey)
		assert.Equal(t, http.StatusForbidden, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "Donate an AI Studio account first to use the AIS PG pool")
		assert.Zero(t, ledgerOf(stranger.Id, wtAisArgon).Id, "a refused member gets no ledger row")

		modelName.Store("gpt-4o")
		assert.Equal(t, http.StatusNoContent, wtAisRelayCall(relay, strangerKey).Code, "other models pass untouched")

		// Each model has its own daily share: 50 argon calls, then 100 flash
		// calls, each ledger untouched by the other model.
		for _, spec := range model.WtAisModelsOf(model.WtAisProvider) {
			modelName.Store(spec.Name)
			for i := range spec.CallsPerAccount {
				recorder = wtAisRelayCall(relay, donorKey)
				require.Equal(t, http.StatusNoContent, recorder.Code, "%s call %d", spec.Upstream, i+1)
				assert.Equal(t, strconv.Itoa(spec.CallsPerAccount-i-1), recorder.Header().Get("X-WT-AIS-Remaining"))
			}
			recorder = wtAisRelayCall(relay, donorKey)
			assert.Equal(t, http.StatusTooManyRequests, recorder.Code, spec.Upstream)
			assert.Contains(t, recorder.Body.String(), "Your AIS PG calls for today are used up")
			ledger := ledgerOf(donor.Id, spec.Key)
			assert.Equal(t, spec.CallsPerAccount, ledger.Calls)
			assert.Zero(t, ledger.Failed)
			if spec.Upstream == wtAisArgon {
				assert.Zero(t, ledgerOf(donor.Id, wtAisFlash).Id, "argon calls do not touch the flash ledger")
			}
		}
		assert.Equal(t, 50, ledgerOf(donor.Id, wtAisArgon).Calls, "flash calls do not touch the argon ledger")

		// A new day starts the count over; a failed upstream request is given
		// back and counted as failed on its model only.
		pinWtClock(t, wtDonationClock.Add(24*time.Hour))
		modelName.Store(wtAisArgonRelay)
		upstreamFails.Store(true)
		recorder = wtAisRelayCall(relay, donorKey)
		assert.Equal(t, http.StatusBadGateway, recorder.Code)
		assert.Equal(t, "49", recorder.Header().Get("X-WT-AIS-Remaining"))
		ledger := ledgerOf(donor.Id, wtAisArgon)
		assert.Zero(t, ledger.Calls)
		assert.Equal(t, 1, ledger.Failed)
		assert.Zero(t, ledgerOf(donor.Id, wtAisFlash).Id)
		upstreamFails.Store(false)
		recorder = wtAisRelayCall(relay, donorKey)
		assert.Equal(t, http.StatusNoContent, recorder.Code)
		assert.Equal(t, "49", recorder.Header().Get("X-WT-AIS-Remaining"))
		modelName.Store(wtAisFlashRelay)
		recorder = wtAisRelayCall(relay, donorKey)
		assert.Equal(t, http.StatusNoContent, recorder.Code)
		assert.Equal(t, "99", recorder.Header().Get("X-WT-AIS-Remaining"))

		recorder = wtAisRelayCall(relay, ownerKey)
		assert.Equal(t, http.StatusNoContent, recorder.Code)
		assert.Empty(t, recorder.Header().Get("X-WT-AIS-Remaining"))
		assert.Zero(t, ledgerOf(owner.Id, wtAisFlash).Id, "the owner is not counted")

		// Off the request path, every finished call moved the member's day
		// counters and streaks: 150 calls on the first day, a failure and two
		// calls on the second; the refused calls and the owner's did not.
		model.WtFlushCallCounters()
		dayOne, dayTwo := model.WtCheckinDay(wtDonationClock), model.WtCheckinDay(wtDonationClock.Add(24*time.Hour))
		counters := func(userID int, day string) model.WtCallDay {
			var row model.WtCallDay
			require.NoError(t, model.DB.Where("user_id = ? AND day = ?", userID, day).Limit(1).Find(&row).Error)
			return row
		}
		streakOf := func(userID int) model.WtCallStreak {
			var row model.WtCallStreak
			require.NoError(t, model.DB.Where("user_id = ?", userID).Limit(1).Find(&row).Error)
			return row
		}
		first := counters(donor.Id, dayOne)
		assert.Equal(t, model.WtCallDay{Id: first.Id, UserId: donor.Id, Day: dayOne, Calls: 150}, first)
		second := counters(donor.Id, dayTwo)
		assert.Equal(t, model.WtCallDay{Id: second.Id, UserId: donor.Id, Day: dayTwo, Calls: 2, Failed: 1}, second)
		streak := streakOf(donor.Id)
		assert.Equal(t, model.WtCallStreak{Id: streak.Id, UserId: donor.Id, SuccessRun: 2, BestSuccess: 150, BestFail: 1, UpdatedAt: wtDonationClock.Add(24 * time.Hour).Unix()}, streak)
		assert.EqualValues(t, 1, counters(stranger.Id, dayOne).Calls, "a call of another model counts too")
		assert.EqualValues(t, 1, streakOf(stranger.Id).BestSuccess)
		assert.Zero(t, counters(owner.Id, dayTwo).Id, "the owner is not counted")
	})
}

// wtAisPoolReply is the shape of the pool page the tests read.
type wtAisPoolReply struct {
	Antigravity *service.WtPoolLevel   `json:"antigravity"`
	Aistudio    *model.WtAisBalance    `json:"aistudio"`
	Build       *model.WtAisBalance    `json:"build"`
	Workbuddy   *service.WtWbPoolLevel `json:"workbuddy"`
	Events      []model.WtPoolEvent    `json:"events"`
}

func wtReadPool(t *testing.T, member service.AuthIdentity) wtAisPoolReply {
	t.Helper()
	_, response := wtAisCall(t, member, WtGetPool, http.MethodGet, "/api/wt/pool", "")
	require.True(t, response.Success, response.Message)
	var reply wtAisPoolReply
	require.NoError(t, common.Unmarshal(response.Data, &reply))
	return reply
}

// Giving moves remaining calls of one model into that model's public pool,
// taking moves them back out within the daily cap, the two models' ledgers
// and pools never mix, non-donors can do neither, an unknown model is
// refused, and every move is announced and audited with its model.
func TestWtAisGiveAndTake(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		newWtFakeAis(t)
		t.Setenv("WT_CPA_URL", "")
		t.Setenv("WT_CPA_MANAGEMENT_KEY", "")
		alice := createWtUser(t, model.User{Username: "alice", DisplayName: "Alice"})
		bob := createWtUser(t, model.User{Username: "bob"})
		carol := createWtUser(t, model.User{Username: "carol"})
		wtAisDonation(t, alice.Id, "acc-a", "a@example.com", 100)
		wtAisDonation(t, bob.Id, "acc-b1", "b1@example.com", 200)
		wtAisDonation(t, bob.Id, "acc-b2", "b2@example.com", 300)
		sessions := map[int]service.AuthIdentity{alice.Id: wtSession(t, alice.Id), bob.Id: wtSession(t, bob.Id), carol.Id: wtSession(t, carol.Id)}
		var move struct {
			Model       string `json:"model"`
			Remaining   int    `json:"remaining"`
			PoolBalance int    `json:"pool_balance"`
			TakeLeft    int    `json:"take_left"`
		}
		give := func(userID int, upstream string, count int) securityEnrollmentResponse {
			_, response := wtAisCall(t, sessions[userID], WtGiveAisCalls, http.MethodPost, "/api/wt/pool/ais/give", wtAisMoveBody(upstream, count))
			if response.Success {
				require.NoError(t, common.Unmarshal(response.Data, &move))
			}
			return response
		}
		take := func(userID int, upstream string, count int) securityEnrollmentResponse {
			_, response := wtAisCall(t, sessions[userID], WtTakeAisCalls, http.MethodPost, "/api/wt/pool/ais/take", wtAisMoveBody(upstream, count))
			if response.Success {
				require.NoError(t, common.Unmarshal(response.Data, &move))
			}
			return response
		}
		const notDonor = "Donate an AI Studio account first to use the AIS PG pool"
		const poolEmpty = "The pool does not hold that many calls right now"

		require.True(t, give(alice.Id, wtAisArgon, 10).Success)
		assert.Equal(t, wtAisArgon, move.Model)
		assert.Equal(t, 40, move.Remaining)
		assert.Equal(t, 10, move.PoolBalance)
		assert.Equal(t, "You can only give calls you still have today", give(alice.Id, wtAisArgon, 41).Message)
		for _, body := range []string{`{"model":"gemini-4-argon","count":0}`, `{"model":"gemini-4-argon","count":-3}`, `{"model":"gemini-4-argon","count":"5"}`, `{"model":"gemini-4-argon","count":1.5}`, `{"model":"gemini-4-argon"}`, `nope`} {
			status, response := wtAisCall(t, sessions[alice.Id], WtGiveAisCalls, http.MethodPost, "/api/wt/pool/ais/give", body)
			assert.Equal(t, http.StatusOK, status, body)
			assert.Equal(t, "Enter a whole number of calls, 1 or more", response.Message, body)
			status, response = wtAisCall(t, sessions[alice.Id], WtTakeAisCalls, http.MethodPost, "/api/wt/pool/ais/take", body)
			assert.Equal(t, http.StatusOK, status, body)
			assert.Equal(t, "Enter a whole number of calls, 1 or more", response.Message, body)
		}
		// The model is the upstream key; a missing, unknown or relay-named
		// model is refused with 400 before the count is looked at.
		for _, body := range []string{`{"model":"gemini-9","count":1}`, `{"count":1}`, `{"model":"AIS PG-gemini-4-argon","count":1}`, `{"model":"","count":0}`} {
			status, response := wtAisCall(t, sessions[alice.Id], WtGiveAisCalls, http.MethodPost, "/api/wt/pool/ais/give", body)
			assert.Equal(t, http.StatusBadRequest, status, body)
			assert.Equal(t, "Unknown AIS PG model", response.Message, body)
			status, response = wtAisCall(t, sessions[bob.Id], WtTakeAisCalls, http.MethodPost, "/api/wt/pool/ais/take", body)
			assert.Equal(t, http.StatusBadRequest, status, body)
			assert.Equal(t, "Unknown AIS PG model", response.Message, body)
		}
		assert.Equal(t, notDonor, take(carol.Id, wtAisArgon, 1).Message)
		assert.Equal(t, notDonor, give(carol.Id, wtAisFlash, 1).Message)

		require.True(t, take(bob.Id, wtAisArgon, 5).Success)
		assert.Equal(t, wtAisArgon, move.Model)
		assert.Equal(t, 105, move.Remaining)
		assert.Equal(t, 5, move.PoolBalance)
		assert.Equal(t, 95, move.TakeLeft)
		assert.Equal(t, poolEmpty, take(bob.Id, wtAisArgon, 6).Message)
		assert.Equal(t, poolEmpty, take(bob.Id, wtAisFlash, 1).Message, "the argon pool holds nothing for flash")
		require.True(t, give(alice.Id, wtAisFlash, 60).Success)
		assert.Equal(t, wtAisFlash, move.Model)
		assert.Equal(t, 40, move.Remaining)
		assert.Equal(t, 60, move.PoolBalance)
		require.True(t, take(bob.Id, wtAisFlash, 60).Success)
		assert.Equal(t, 260, move.Remaining)
		assert.Equal(t, 0, move.PoolBalance)
		assert.Equal(t, 40, move.TakeLeft)
		require.True(t, give(alice.Id, wtAisArgon, 30).Success)
		assert.Equal(t, 10, move.Remaining)
		assert.Equal(t, 35, move.PoolBalance)
		response := take(bob.Id, wtAisArgon, 96)
		assert.Equal(t, "You can take at most 100 calls a day from the pool", response.Message)
		assert.Equal(t, "You can take at most {{max}} calls a day from the pool", response.MessageKey)
		assert.Equal(t, "You can take at most 100 calls a day from the pool", take(bob.Id, wtAisArgon, 101).Message)
		require.True(t, take(bob.Id, wtAisArgon, 35).Success)
		assert.Equal(t, 140, move.Remaining)
		assert.Equal(t, 0, move.PoolBalance)
		assert.Equal(t, 60, move.TakeLeft)

		// The gate follows the ledger of each model: Alice has ten argon calls
		// left today and forty flash calls.
		day := model.WtCheckinDay(wtDonationClock)
		for range 10 {
			_, err := model.WtAisReserveCall(alice.Id, day, wtAisArgon)
			require.NoError(t, err)
		}
		_, err := model.WtAisReserveCall(alice.Id, day, wtAisArgon)
		assert.ErrorIs(t, err, model.ErrWtAisExhausted)
		remaining, err := model.WtAisReserveCall(alice.Id, day, wtAisFlash)
		require.NoError(t, err)
		assert.Equal(t, 39, remaining)
		_, err = model.WtAisReserveCall(alice.Id, day, "gemini-9")
		assert.ErrorIs(t, err, model.ErrWtAisUnknownModel)
		assert.Equal(t, "You can only give calls you still have today", give(alice.Id, wtAisArgon, 1).Message)

		pool := wtReadPool(t, sessions[alice.Id])
		assert.Nil(t, pool.Antigravity)
		require.NotNil(t, pool.Aistudio)
		assert.Equal(t, "2026-10-15", pool.Aistudio.Day)
		assert.Equal(t, 1, pool.Aistudio.Accounts)
		assert.Equal(t, model.WtAisModelBalance{Provider: model.WtAisProvider, Model: wtAisArgon, RelayModel: wtAisArgonRelay, PerAccount: 50, TakeMax: 100, Allowance: 10, Calls: 10, Given: 40, Remaining: 0, TakeLeft: 100, PoolGiven: 40, PoolTaken: 40}, wtAisModelOf(t, pool.Aistudio, wtAisArgon))
		assert.Equal(t, model.WtAisModelBalance{Provider: model.WtAisProvider, Model: wtAisFlash, RelayModel: wtAisFlashRelay, PerAccount: 100, TakeMax: 100, Allowance: 40, Calls: 1, Given: 60, Remaining: 39, TakeLeft: 100, PoolGiven: 60, PoolTaken: 60}, wtAisModelOf(t, pool.Aistudio, wtAisFlash))
		pool = wtReadPool(t, sessions[bob.Id])
		assert.Equal(t, 2, pool.Aistudio.Accounts)
		assert.Equal(t, model.WtAisModelBalance{Provider: model.WtAisProvider, Model: wtAisArgon, RelayModel: wtAisArgonRelay, PerAccount: 50, TakeMax: 100, Allowance: 140, Taken: 40, Remaining: 140, TakeLeft: 60, PoolGiven: 40, PoolTaken: 40}, wtAisModelOf(t, pool.Aistudio, wtAisArgon))
		assert.Equal(t, model.WtAisModelBalance{Provider: model.WtAisProvider, Model: wtAisFlash, RelayModel: wtAisFlashRelay, PerAccount: 100, TakeMax: 100, Allowance: 260, Taken: 60, Remaining: 260, TakeLeft: 40, PoolGiven: 60, PoolTaken: 60}, wtAisModelOf(t, pool.Aistudio, wtAisFlash))
		require.Len(t, pool.Events, 6)
		for i, want := range []struct {
			kind, name, model string
			amount            int
		}{
			{"ais_taken", "bob", wtAisArgon, 35},
			{"ais_given", "Alice", wtAisArgon, 30},
			{"ais_taken", "bob", wtAisFlash, 60},
			{"ais_given", "Alice", wtAisFlash, 60},
			{"ais_taken", "bob", wtAisArgon, 5},
			{"ais_given", "Alice", wtAisArgon, 10},
		} {
			assert.Equal(t, want.kind, pool.Events[i].Kind, i)
			assert.Equal(t, want.name, pool.Events[i].DisplayName, i)
			assert.Equal(t, want.model, pool.Events[i].Model, i)
			assert.Equal(t, want.amount, pool.Events[i].Amount, i)
			assert.Equal(t, model.WtAisProvider, pool.Events[i].Provider, i)
		}

		gives := wtAudits(t, "wt.ais_give")
		require.Len(t, gives, 6, "every give that reached the ledger is audited")
		assert.Contains(t, wtAuditParams(t, gives[0]), `"count":10`)
		assert.Contains(t, wtAuditParams(t, gives[0]), `"model":"gemini-4-argon"`)
		assert.Contains(t, wtAuditParams(t, gives[0]), `"success":true`)
		assert.Contains(t, wtAuditParams(t, gives[1]), `"success":false`)
		assert.Contains(t, wtAuditParams(t, gives[3]), `"model":"gemini-3.8-flash"`)
		takes := wtAudits(t, "wt.ais_take")
		require.Len(t, takes, 8)
		assert.Contains(t, wtAuditParams(t, takes[0]), `"success":false`)
		assert.Contains(t, wtAuditParams(t, takes[1]), `"count":5`)
		assert.Contains(t, wtAuditParams(t, takes[4]), `"model":"gemini-3.8-flash"`)

		// Nothing carries over to the next day.
		pinWtClock(t, wtDonationClock.Add(24*time.Hour))
		require.True(t, give(alice.Id, wtAisArgon, 50).Success)
		assert.Equal(t, 0, move.Remaining)
		assert.Equal(t, 50, move.PoolBalance)
		assert.Equal(t, poolEmpty, take(bob.Id, wtAisArgon, 51).Message)
	})
}

// The feed keeps the newest 200 announcements, cuts long names, and serves
// what is newer than an id.
func TestWtPoolEvents(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		newWtFakeAis(t)
		alice := createWtUser(t, model.User{Username: "alice", DisplayName: "Alice"})
		dave := createWtUser(t, model.User{Username: "dave"})

		longName := strings.Repeat("名", 70)
		for i := range 205 {
			require.NoError(t, model.WtInsertPoolEvent(model.WtPoolEventAisGiven, model.WtAisProvider, alice.Id, longName, i+1, wtAisArgon))
		}
		var count int64
		require.NoError(t, model.DB.Model(&model.WtPoolEvent{}).Count(&count).Error)
		assert.EqualValues(t, model.WtPoolEventsKeep, count, "the feed keeps the newest 200")
		events := wtPoolEvents(t)
		require.Len(t, events, 50, "a page is 50")
		assert.Equal(t, 205, events[0].Amount, "newest first")
		assert.Equal(t, wtAisArgon, events[0].Model)
		assert.Equal(t, strings.Repeat("名", 64), events[0].DisplayName, "names are cut to 64 runes")
		var oldest model.WtPoolEvent
		require.NoError(t, model.DB.Order("id ASC").First(&oldest).Error)
		assert.Equal(t, 6, oldest.Amount, "the five oldest are gone")

		newer, err := model.WtListPoolEvents(events[3].Id, 50)
		require.NoError(t, err)
		require.Len(t, newer, 3)
		assert.Equal(t, events[0].Id, newer[0].Id)

		_, response := wtAisCall(t, wtSession(t, dave.Id), WtListPoolEvents, http.MethodGet, "/api/wt/pool/events?after="+strconv.Itoa(events[1].Id), "")
		require.True(t, response.Success, response.Message)
		var feed struct {
			Items []model.WtPoolEvent `json:"items"`
		}
		require.NoError(t, common.Unmarshal(response.Data, &feed))
		require.Len(t, feed.Items, 1)
		assert.Equal(t, events[0].Id, feed.Items[0].Id)
		raw, err := common.Marshal(feed.Items[0])
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "user_id", "the feed names members, it does not number them")
		assert.Contains(t, string(raw), `"model":"gemini-4-argon"`)
		_, response = wtAisCall(t, wtSession(t, dave.Id), WtListPoolEvents, http.MethodGet, "/api/wt/pool/events?after=abc", "")
		require.NoError(t, common.Unmarshal(response.Data, &feed))
		assert.Len(t, feed.Items, 50, "a bad after is the latest page")
	})
}

// Every start puts the AIS group, the ratios of both AIS models and the
// wt-ais channel back the way the site wants them, and leaves everything
// else the owner set alone.
func TestWtEnsureAisChannel(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		common.OptionMapRWMutex.RLock()
		previousOptions := common.OptionMap
		common.OptionMapRWMutex.RUnlock()
		previousGroups, previousUsable := ratio_setting.GroupRatio2JSONString(), setting.UserUsableGroups2JSONString()
		previousModels, previousCompletion := ratio_setting.ModelRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
		t.Cleanup(func() {
			common.OptionMapRWMutex.Lock()
			common.OptionMap = previousOptions
			common.OptionMapRWMutex.Unlock()
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroups))
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsable))
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModels))
			require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(previousCompletion))
		})
		model.InitOptionMap()
		for _, name := range []string{"WT_AIS_URL", "WT_AIS_PASSWORD", "WT_AIS_API_KEY"} {
			t.Setenv(name, "")
		}
		require.NoError(t, service.WtEnsureAisChannel(), "an unconfigured pool is left alone")
		channels, err := model.GetChannelsByTag(service.WtAisChannelTag, true, true)
		require.NoError(t, err)
		assert.Empty(t, channels)

		t.Setenv("WT_AIS_URL", "http://aistudio-api:8080/")
		t.Setenv("WT_AIS_PASSWORD", wtTestAisPassword)
		t.Setenv("WT_AIS_API_KEY", wtTestAisKey)
		setRatios := func(option string, values map[string]float64) {
			encoded, err := common.Marshal(values)
			require.NoError(t, err)
			require.NoError(t, model.UpdateOption(option, string(encoded)))
		}
		modelRatios := ratio_setting.GetModelRatioCopy()
		modelRatios[wtAisArgon], modelRatios[wtAisFlash] = 2.5, 0.5
		setRatios("ModelRatio", modelRatios)
		completionRatios := ratio_setting.GetCompletionRatioCopy()
		completionRatios[wtAisArgon] = 4
		delete(completionRatios, wtAisFlash)
		setRatios("CompletionRatio", completionRatios)

		for range 2 {
			require.NoError(t, service.WtEnsureAisChannel())
		}
		assert.Equal(t, 1.0, ratio_setting.GetGroupRatioCopy()[service.WtAisGroup])
		assert.Equal(t, "AIS playground pool", setting.GetUserUsableGroupsCopy()[service.WtAisGroup])
		assert.Equal(t, 2.5, ratio_setting.GetModelRatioCopy()[wtAisArgonRelay], "each relay model bills like its upstream model")
		assert.Equal(t, 0.5, ratio_setting.GetModelRatioCopy()[wtAisFlashRelay])
		assert.Equal(t, 4.0, ratio_setting.GetCompletionRatioCopy()[wtAisArgonRelay])
		assert.Equal(t, 1.0, ratio_setting.GetCompletionRatioCopy()[wtAisFlashRelay], "a model without a ratio of its own bills at 1")
		for key, want := range map[string]map[string]any{
			"GroupRatio":       {service.WtAisGroup: 1.0},
			"UserUsableGroups": {service.WtAisGroup: service.WtAisGroupLabel},
			"ModelRatio":       {wtAisArgonRelay: 2.5, wtAisFlashRelay: 0.5},
			"CompletionRatio":  {wtAisArgonRelay: 4.0, wtAisFlashRelay: 1.0},
		} {
			var option model.Option
			require.NoError(t, model.DB.Where(&model.Option{Key: key}).First(&option).Error, key)
			var values map[string]any
			require.NoError(t, common.UnmarshalJsonStr(option.Value, &values), key)
			for field, value := range want {
				assert.Equal(t, value, values[field], key+" "+field)
			}
		}
		channels, err = model.GetChannelsByTag(service.WtAisChannelTag, true, true)
		require.NoError(t, err)
		require.Len(t, channels, 1, "two starts create one channel")
		channel := channels[0]
		assert.Equal(t, constant.ChannelTypeGemini, channel.Type)
		assert.Equal(t, wtTestAisKey, channel.Key)
		assert.Equal(t, "http://aistudio-api:8080", channel.GetBaseURL())
		assert.Equal(t, "AIS PG-gemini-4-argon,AIS PG-gemini-3.8-flash", channel.Models, "the channel carries both models")
		assert.Equal(t, "default,vip,svip,AIS PG", channel.Group)
		var mapping map[string]string
		require.NoError(t, common.UnmarshalJsonStr(channel.GetModelMapping(), &mapping))
		assert.Equal(t, map[string]string{wtAisArgonRelay: wtAisArgon, wtAisFlashRelay: wtAisFlash}, mapping)
		assert.Equal(t, service.WtAisChannelName, channel.Name)
		assert.Equal(t, common.ChannelStatusEnabled, channel.Status)
		assert.Zero(t, channel.GetPriority())
		assert.Zero(t, channel.GetWeight())
		var abilities []model.Ability
		require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
		pairs := make([]string, 0, len(abilities))
		for _, ability := range abilities {
			assert.True(t, ability.Enabled)
			pairs = append(pairs, ability.Group+"|"+ability.Model)
		}
		want := []string{}
		for _, group := range []string{"default", "vip", "svip", "AIS PG"} {
			for _, spec := range model.WtAisModelsOf(model.WtAisProvider) {
				want = append(want, group+"|"+spec.Name)
			}
		}
		assert.ElementsMatch(t, want, pairs, "every group can call both models")

		// The owner's edits of the owned fields revert on the next start; a
		// remark and a changed group ratio stay.
		remark := "owner note"
		require.NoError(t, model.DB.Model(channel).Updates(map[string]any{"priority": 5, "status": common.ChannelStatusManuallyDisabled, "name": "renamed", "models": wtAisArgonRelay, "remark": remark}).Error)
		groupRatios := ratio_setting.GetGroupRatioCopy()
		groupRatios[service.WtAisGroup] = 0.5
		setRatios("GroupRatio", groupRatios)
		modelRatios = ratio_setting.GetModelRatioCopy()
		modelRatios[wtAisArgonRelay] = 9
		setRatios("ModelRatio", modelRatios)
		t.Setenv("WT_AIS_API_KEY", "rotated-relay-key")
		require.NoError(t, service.WtEnsureAisChannel())
		channel, err = model.GetChannelById(channel.Id, true)
		require.NoError(t, err)
		assert.Zero(t, channel.GetPriority())
		assert.Equal(t, common.ChannelStatusEnabled, channel.Status)
		assert.Equal(t, service.WtAisChannelName, channel.Name)
		assert.Equal(t, "AIS PG-gemini-4-argon,AIS PG-gemini-3.8-flash", channel.Models)
		assert.Equal(t, "rotated-relay-key", channel.Key)
		require.NotNil(t, channel.Remark)
		assert.Equal(t, remark, *channel.Remark)
		assert.Equal(t, 2.5, ratio_setting.GetModelRatioCopy()[wtAisArgonRelay], "a relay model's ratio follows its upstream model")
		assert.Equal(t, 0.5, ratio_setting.GetGroupRatioCopy()[service.WtAisGroup], "an existing group ratio is the owner's")
	})
}

// The Antigravity tank averages Google's remaining fractions over every
// credential of the pool that answers, counts the healthy ones, and is
// empty before the first refresh.
func TestWtPoolLevelSnapshot(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeCPA(t)
		for _, name := range []string{"WT_AIS_URL", "WT_AIS_PASSWORD", "WT_AIS_API_KEY"} {
			t.Setenv(name, "")
		}
		member := createWtUser(t, model.User{Username: "donor"})
		session := wtSession(t, member.Id)
		bucket := func(fiveHours, weekly float64) string {
			return fmt.Sprintf(`{"groups":[{"displayName":"Gemini 3 Pro","buckets":[{"displayName":"Gemini 3 Pro","window":"5h","remainingFraction":%g},{"displayName":"Gemini 3 Pro","window":"weekly","remainingFraction":%g}]}]}`, fiveHours, weekly)
		}
		fake.addCredential("a@example.com", nil)
		fake.addCredential("b@example.com", nil)
		fake.addCredential("c@example.com", map[string]any{"status": "error", "status_message": "quota exhausted", "next_retry_after": wtDonationClock.Add(20 * time.Minute).Format(time.RFC3339)})
		fake.addCredential("d@example.com", map[string]any{"project_id": ""})
		fake.addCredential("e@example.com", map[string]any{"provider": "gemini-cli"})
		fake.mu.Lock()
		fake.quotaByIndex = map[string]string{"idx-a@example.com": bucket(0.8, 0.5), "idx-b@example.com": bucket(0.4, 0.3), "idx-c@example.com": bucket(0.2, 0.1)}
		fake.mu.Unlock()

		pool := wtReadPool(t, session)
		require.NotNil(t, pool.Antigravity)
		assert.Equal(t, service.WtPoolLevel{Buckets: []service.WtPoolLevelBucket{}}, *pool.Antigravity, "nothing before the first refresh")
		assert.Nil(t, pool.Aistudio)

		service.WtRefreshPoolLevel(context.Background())
		pool = wtReadPool(t, session)
		require.NotNil(t, pool.Antigravity)
		assert.Equal(t, wtDonationClock.Unix(), pool.Antigravity.FetchedAt)
		assert.Equal(t, 4, pool.Antigravity.Accounts, "other providers are not Antigravity accounts")
		assert.Equal(t, 3, pool.Antigravity.Healthy)
		require.Len(t, pool.Antigravity.Buckets, 2)
		assert.Equal(t, "Gemini 3 Pro", pool.Antigravity.Buckets[0].Name)
		assert.Equal(t, "5h", pool.Antigravity.Buckets[0].Window)
		assert.InDelta(t, (0.8+0.4+0.2)/3, pool.Antigravity.Buckets[0].AverageRemainingFraction, 0.0001)
		assert.Equal(t, "weekly", pool.Antigravity.Buckets[1].Window)
		assert.InDelta(t, (0.5+0.3+0.1)/3, pool.Antigravity.Buckets[1].AverageRemainingFraction, 0.0001)
		calls := fake.requestsOf("POST /v8/management/requests/api-call")
		assert.Equal(t, 6, calls, "quota and tier once per credential with a project")

		service.WtRefreshPoolLevel(context.Background())
		assert.Equal(t, calls, fake.requestsOf("POST /v8/management/requests/api-call"), "the usage cache serves the next refresh")

		fake.mu.Lock()
		fake.down = true
		fake.mu.Unlock()
		service.WtForgetPoolCredentials()
		service.WtRefreshPoolLevel(context.Background())
		assert.Equal(t, 4, wtReadPool(t, session).Antigravity.Accounts, "an unreachable pool keeps the last snapshot")
	})
}
