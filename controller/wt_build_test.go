package controller

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
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

// wind-tide: regression tests of the BUILD pool (wind-tide/API.md §15)
// against a scripted stand-in for the AIStudioToAPI console: its session
// sign-in, its status, the upload of auth files, the switch and the delete.

const (
	wtTestBuildPassword = "test-build-console-password"
	wtTestBuildKey      = "test-build-relay-key"
	wtBuildFlashRelay   = "Build-gemini-3.8-flash"
	wtBuildSessionName  = "build.sid"
)

// wtFakeBuildAccount is one auth file as the console would report it.
type wtFakeBuildAccount struct {
	name       string
	hasContext bool
	expired    bool
	invalid    bool
	// expireOnSwitch makes the sign-in bounce when the console is asked to
	// serve the account; vanish loses the file before the first status read;
	// busyAfterUpload leaves the console switching once the file is saved.
	expireOnSwitch  bool
	vanish          bool
	busyAfterUpload bool
}

// wtFakeBuild plays the AIStudioToAPI console for one test: a password
// sign-in that hands out session cookies, a status that lists the auth files,
// uploads that number them, switches that sign them in and deletes.
type wtFakeBuild struct {
	t      *testing.T
	server *httptest.Server
	mu     sync.Mutex

	down     bool
	busy     bool
	sessions map[string]bool
	logins   int
	accounts map[int]*wtFakeBuildAccount
	next     *wtFakeBuildAccount // the flags of the next uploaded account
	current  int
	uploads  []map[string]any
	switches []int
	deleted  []int
	requests []string
}

func newWtFakeBuild(t *testing.T) *wtFakeBuild {
	t.Helper()
	fake := &wtFakeBuild{t: t, sessions: map[string]bool{}, accounts: map[int]*wtFakeBuildAccount{}, current: -1}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	t.Setenv("WT_BUILD_URL", fake.server.URL)
	t.Setenv("WT_BUILD_PASSWORD", wtTestBuildPassword)
	t.Setenv("WT_BUILD_API_KEY", wtTestBuildKey)
	interval, inline := service.WtBuildVerifyInterval, service.WtDonationVerifySync
	service.WtBuildVerifyInterval, service.WtDonationVerifySync = time.Millisecond, true
	t.Cleanup(func() {
		service.WtBuildVerifyInterval, service.WtDonationVerifySync = interval, inline
	})
	service.WtResetDonationCaches()
	t.Cleanup(service.WtResetDonationCaches)
	return fake
}

func (fake *wtFakeBuild) handle(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	route := r.Method + " " + r.URL.Path
	fake.requests = append(fake.requests, route)
	if fake.down {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if route == "POST /login" {
		var body map[string]string
		_ = common.DecodeJson(r.Body, &body)
		fake.logins++
		if body["password"] != wtTestBuildPassword {
			w.Header().Set("Location", "/login?error=1")
			w.WriteHeader(http.StatusFound)
			return
		}
		sid := fmt.Sprintf("sid-%d", fake.logins)
		fake.sessions[sid] = true
		http.SetCookie(w, &http.Cookie{Name: wtBuildSessionName, Value: sid, Path: "/", HttpOnly: true})
		w.Header().Set("Location", "/")
		w.WriteHeader(http.StatusFound)
		return
	}
	if cookie, err := r.Cookie(wtBuildSessionName); err != nil || !fake.sessions[cookie.Value] {
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			wtJSONReply(w, http.StatusUnauthorized, map[string]any{"message": "unlimited"})
			return
		}
		w.Header().Set("Location", "/login")
		w.WriteHeader(http.StatusSeeOther)
		return
	}
	assert.Equal(fake.t, "application/json", r.Header.Get("Accept"))
	switch {
	case route == "GET /api/status":
		details := make([]map[string]any, 0, len(fake.accounts))
		for _, index := range slices.Sorted(maps.Keys(fake.accounts)) {
			account := fake.accounts[index]
			if account.vanish {
				delete(fake.accounts, index)
				continue
			}
			details = append(details, map[string]any{
				"canonicalIndex": index, "hasContext": account.hasContext, "index": index, "isDuplicate": false,
				"isExpired": account.expired, "isInvalid": account.invalid, "isRotation": !account.expired && !account.invalid, "name": account.name,
			})
		}
		wtJSONReply(w, http.StatusOK, map[string]any{"status": map[string]any{"accountDetails": details, "currentAuthIndex": fake.current, "isSystemBusy": fake.busy, "maxContexts": 2, "usageCount": 0}})
	case route == "POST /api/files":
		if fake.busy {
			wtJSONReply(w, http.StatusConflict, map[string]any{"message": "busy"})
			return
		}
		var body map[string]any
		assert.NoError(fake.t, common.DecodeJson(r.Body, &body))
		content, _ := body["content"].(map[string]any)
		if content == nil {
			wtJSONReply(w, http.StatusBadRequest, map[string]any{"message": "content required"})
			return
		}
		fake.uploads = append(fake.uploads, content)
		index := 0
		for existing := range fake.accounts {
			index = max(index, existing+1)
		}
		account := wtFakeBuildAccount{hasContext: true}
		if fake.next != nil {
			account, fake.next = *fake.next, nil
		}
		account.name, _ = content["accountName"].(string)
		fake.accounts[index] = &account
		if account.busyAfterUpload {
			fake.busy = true
		}
		wtJSONReply(w, http.StatusOK, map[string]any{"filename": fmt.Sprintf("auth-%d.json", index)})
	case route == "PUT /api/accounts/current":
		var body map[string]int
		assert.NoError(fake.t, common.DecodeJson(r.Body, &body))
		index := body["targetIndex"]
		fake.switches = append(fake.switches, index)
		account := fake.accounts[index]
		switch {
		case fake.busy:
			wtJSONReply(w, http.StatusConflict, map[string]any{"message": "busy"})
		case account == nil:
			wtJSONReply(w, http.StatusBadRequest, map[string]any{"message": "accountSwitchFailed"})
		default:
			fake.current = index
			if account.expireOnSwitch {
				account.expired = true
			} else {
				account.hasContext = true
			}
			wtJSONReply(w, http.StatusOK, map[string]any{"message": "accountSwitchSuccess", "newIndex": index})
		}
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/accounts/"):
		index, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/accounts/"))
		assert.NoError(fake.t, err)
		assert.Equal(fake.t, "true", r.URL.Query().Get("force"), "the served account is deleted too")
		fake.deleted = append(fake.deleted, index)
		switch {
		case fake.busy:
			wtJSONReply(w, http.StatusConflict, map[string]any{"message": "busy"})
		case fake.accounts[index] == nil:
			wtJSONReply(w, http.StatusNotFound, map[string]any{"message": "not found"})
		default:
			delete(fake.accounts, index)
			if fake.current == index {
				fake.current = -1
			}
			wtJSONReply(w, http.StatusOK, map[string]any{"message": "deleted"})
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// set changes the fake under its lock.
func (fake *wtFakeBuild) set(change func(fake *wtFakeBuild)) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	change(fake)
}

// hold puts an auth file in the console without an upload.
func (fake *wtFakeBuild) hold(index int, account wtFakeBuildAccount) {
	fake.set(func(fake *wtFakeBuild) {
		held := account
		fake.accounts[index] = &held
	})
}

// scriptUpload sets the flags of the next uploaded account.
func (fake *wtFakeBuild) scriptUpload(account wtFakeBuildAccount) {
	fake.set(func(fake *wtFakeBuild) {
		next := account
		fake.next = &next
	})
}

// requestsOf counts the calls of one route, "METHOD /path".
func (fake *wtFakeBuild) requestsOf(route string) int {
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

func (fake *wtFakeBuild) loginsSeen() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.logins
}

func (fake *wtFakeBuild) uploadsSeen() []map[string]any {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return slices.Clone(fake.uploads)
}

func (fake *wtFakeBuild) switchesSeen() []int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return slices.Clone(fake.switches)
}

func (fake *wtFakeBuild) deletedSeen() []int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return slices.Clone(fake.deleted)
}

// wtBuildDonate posts a Cookie to the BUILD pool with the consent given.
func wtBuildDonate(t *testing.T, member service.AuthIdentity, cookies string) (int, securityEnrollmentResponse) {
	t.Helper()
	body, err := common.Marshal(map[string]any{"cookies": cookies, "accepted": true})
	require.NoError(t, err)
	return wtAisCall(t, member, WtDonateBuild, http.MethodPost, "/api/wt/donations/build", string(body))
}

// wtBuildDonation puts an active Build donation of the auth file index in the
// database, as a finished verification would.
func wtBuildDonation(t *testing.T, userID int, index int, createdAt int64) *model.WtDonation {
	t.Helper()
	donation := &model.WtDonation{
		UserId:         userID,
		Provider:       model.WtBuildProvider,
		Status:         model.WtDonationActive,
		CredentialName: strconv.Itoa(index),
		AuthIndex:      wtFingerprintOf(fmt.Sprintf("build-%d-%d", userID, index)),
		CreatedAt:      createdAt,
		SubmittedAt:    createdAt,
		VerifiedAt:     createdAt,
	}
	require.NoError(t, model.DB.Create(donation).Error)
	return donation
}

// Donating a Build account: the consent and the Cookie are checked before the
// console is asked, the Cookie becomes an auth file named after the member,
// and the verification follows the console's status until the account signed
// in, failed to, or was waited for long enough.
func TestWtBuildDonateFlow(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeBuild(t)
		for _, name := range []string{"WT_AIS_URL", "WT_AIS_PASSWORD", "WT_AIS_API_KEY"} {
			t.Setenv(name, "")
		}
		member := createWtUser(t, model.User{Username: "builder", DisplayName: "Bea"})
		session := wtSession(t, member.Id)
		const cookieError = "Paste the Cookie of a signed-in AI Studio page"

		flagsBody := decodeSecurityEnrollmentResponse(t, wtServe(GetStatus, http.MethodGet, "/api/status", "", 0))
		var flags map[string]any
		require.NoError(t, common.Unmarshal(flagsBody.Data, &flags))
		assert.Equal(t, true, flags["wt_build_enabled"])
		assert.Equal(t, false, flags["wt_ais_enabled"])

		// The consent and the Cookie are checked before the console is asked.
		code, response := wtAisCall(t, session, WtDonateBuild, http.MethodPost, "/api/wt/donations/build", `{"cookies":"SID=abc"}`)
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Equal(t, "Accept the risks before donating an account", response.Message)
		code, response = wtAisCall(t, session, WtDonateBuild, http.MethodPost, "/api/wt/donations/build", `{"cookies":`)
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Equal(t, "Invalid parameters", response.Message)
		for _, bad := range []string{"HSID=x; APISID=y", "", "SID=" + strings.Repeat("a", service.WtAisCookieMaxBytes)} {
			code, response = wtBuildDonate(t, session, bad)
			assert.Equal(t, http.StatusBadRequest, code, bad)
			assert.Equal(t, cookieError, response.Message, bad)
		}
		assert.Zero(t, fake.requestsOf("POST /login"), "nothing reached the console")

		// A console busy switching accounts or not answering refuses the
		// upload; neither leaves a row.
		fake.set(func(fake *wtFakeBuild) { fake.busy = true })
		code, response = wtBuildDonate(t, session, "SID=busy")
		assert.Equal(t, http.StatusServiceUnavailable, code)
		assert.Equal(t, "The pool is busy switching accounts; try again in a moment", response.Message)
		fake.set(func(fake *wtFakeBuild) { fake.busy, fake.down = false, true })
		code, response = wtBuildDonate(t, session, "SID=down")
		assert.Equal(t, http.StatusBadGateway, code)
		assert.Equal(t, "The pool is not answering; try again later", response.Message)
		fake.set(func(fake *wtFakeBuild) { fake.down = false })
		var count int64
		require.NoError(t, model.DB.Model(&model.WtDonation{}).Count(&count).Error)
		assert.Zero(t, count, "a refused upload leaves no row")

		// Success: the Cookie becomes an auth file named after the member, the
		// console reports a context for it and the donation goes active.
		code, response = wtBuildDonate(t, session, `[{"name":"__Secure-1PSID","value":"secret-1"},{"name":"SAPISID","value":"s"},{"name":"__Host-GAPS","value":"g"}]`)
		require.Equal(t, http.StatusAccepted, code, response.Message)
		require.True(t, response.Success)
		var accepted struct {
			ID     int    `json:"id"`
			Status string `json:"status"`
		}
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		assert.Equal(t, model.WtDonationVerifying, accepted.Status)
		row := wtDonationRow(t, accepted.ID)
		assert.Equal(t, model.WtBuildProvider, row.Provider)
		assert.Equal(t, model.WtDonationActive, row.Status)
		assert.Empty(t, row.Email, "the console reads no email")
		assert.Equal(t, "0", row.CredentialName, "the index of the auth file")
		assert.Equal(t, wtFingerprintOf("secret-1"), row.AuthIndex)
		assert.Equal(t, wtDonationClock.Unix(), row.VerifiedAt)
		uploads := fake.uploadsSeen()
		require.Len(t, uploads, 1, "the busy console saw the upload, the one that was down did not")
		state := uploads[0]
		assert.Regexp(t, regexp.MustCompile(`^wt-`+strconv.Itoa(member.Id)+`-[0-9a-f]{4}$`), state["accountName"])
		assert.Equal(t, []any{}, state["origins"])
		cookies, _ := state["cookies"].([]any)
		require.Len(t, cookies, 2, "__Host- cookies stay out")
		first, _ := cookies[0].(map[string]any)
		assert.Equal(t, "__Secure-1PSID", first["name"])
		assert.Equal(t, "secret-1", first["value"])
		assert.Equal(t, ".google.com", first["domain"])
		assert.Equal(t, "/", first["path"])
		assert.Equal(t, true, first["httpOnly"])
		assert.Equal(t, true, first["secure"])
		assert.Equal(t, "None", first["sameSite"])
		second, _ := cookies[1].(map[string]any)
		assert.Equal(t, "SAPISID", second["name"])
		assert.Equal(t, false, second["httpOnly"], "the page reads SAPISID from script")
		events := wtPoolEvents(t)
		require.Len(t, events, 1)
		assert.Equal(t, model.WtPoolEventBuildDonated, events[0].Kind)
		assert.Equal(t, model.WtBuildProvider, events[0].Provider)
		assert.Equal(t, "Bea", events[0].DisplayName)
		assert.Equal(t, 1, events[0].Amount)
		assert.Empty(t, events[0].Model, "account events carry no model")

		// The same account again is refused before the console is asked.
		uploadsBefore := fake.requestsOf("POST /api/files")
		code, response = wtBuildDonate(t, session, "__Secure-1PSID=secret-1; SID=something-else")
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "This account is already in the pool", response.Message)
		assert.Equal(t, uploadsBefore, fake.requestsOf("POST /api/files"))

		// The console restarting forgets the session: the next call signs in
		// again and goes through.
		fake.set(func(fake *wtFakeBuild) { fake.sessions = map[string]bool{} })
		logins := fake.loginsSeen()
		_, response = wtBuildDonate(t, session, "SID=secret-2")
		require.True(t, response.Success, response.Message)
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		assert.Equal(t, model.WtDonationActive, wtDonationRow(t, accepted.ID).Status)
		assert.Equal(t, "1", wtDonationRow(t, accepted.ID).CredentialName)
		assert.Equal(t, logins+1, fake.loginsSeen(), "one fresh sign-in")

		// A wrong console password is the pool not answering, not a crash.
		fake.set(func(fake *wtFakeBuild) { fake.sessions = map[string]bool{} })
		t.Setenv("WT_BUILD_PASSWORD", "wrong")
		code, response = wtBuildDonate(t, session, "SID=secret-wrong")
		assert.Equal(t, http.StatusBadGateway, code)
		assert.Equal(t, "The pool is not answering; try again later", response.Message)
		t.Setenv("WT_BUILD_PASSWORD", wtTestBuildPassword)

		// An account the console does not try by itself is asked for by a
		// switch half-way through the wait, which signs it in.
		fake.scriptUpload(wtFakeBuildAccount{})
		_, response = wtBuildDonate(t, session, "SID=secret-3")
		require.True(t, response.Success, response.Message)
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		assert.Equal(t, model.WtDonationActive, wtDonationRow(t, accepted.ID).Status)
		assert.Equal(t, []int{2}, fake.switchesSeen())

		// A sign-in that bounces to Google's login marks the file expired: the
		// donation fails and the file is deleted.
		fake.scriptUpload(wtFakeBuildAccount{expireOnSwitch: true})
		_, response = wtBuildDonate(t, session, "SID=secret-4")
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		row = wtDonationRow(t, accepted.ID)
		assert.Equal(t, model.WtDonationFailed, row.Status)
		assert.Equal(t, service.WtBuildFailExpired, row.FailReason)
		assert.Equal(t, []int{3}, fake.deletedSeen())

		// A file the console could not read fails at once; one the console
		// lost fails as gone; failed donations are not announced.
		fake.scriptUpload(wtFakeBuildAccount{invalid: true})
		_, response = wtBuildDonate(t, session, "SID=secret-5")
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		assert.Equal(t, service.WtBuildFailExpired, wtDonationRow(t, accepted.ID).FailReason)
		fake.scriptUpload(wtFakeBuildAccount{vanish: true})
		_, response = wtBuildDonate(t, session, "SID=secret-6")
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		row = wtDonationRow(t, accepted.ID)
		assert.Equal(t, model.WtDonationFailed, row.Status)
		assert.Equal(t, service.WtBuildFailGone, row.FailReason)
		assert.Len(t, wtPoolEvents(t), 3)

		// A console that stays busy cannot be asked to switch; an account it
		// never tried is taken on trust once the wait is over.
		fake.scriptUpload(wtFakeBuildAccount{busyAfterUpload: true})
		statusReads := fake.requestsOf("GET /api/status")
		switches := len(fake.switchesSeen())
		_, response = wtBuildDonate(t, session, "SID=secret-7")
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		assert.Equal(t, model.WtDonationActive, wtDonationRow(t, accepted.ID).Status)
		assert.Equal(t, statusReads+service.WtBuildVerifyAttempts, fake.requestsOf("GET /api/status"))
		assert.Len(t, fake.switchesSeen(), switches, "a busy console is not asked to switch")
		fake.set(func(fake *wtFakeBuild) { fake.busy = false })

		// Ten verifying or active Build donations fill the slots; AIS PG and
		// Antigravity donations do not count.
		for i := range 6 {
			require.NoError(t, model.DB.Create(&model.WtDonation{UserId: member.Id, Provider: model.WtBuildProvider, Status: model.WtDonationVerifying, CredentialName: strconv.Itoa(20 + i), AuthIndex: fmt.Sprintf("slot-fp-%d", i)}).Error)
		}
		require.NoError(t, model.DB.Create(&model.WtDonation{UserId: member.Id, Provider: model.WtAisProvider, Email: "ais@example.com", Status: model.WtDonationActive, CredentialName: "ais-acc-9", AuthIndex: "ais-fp"}).Error)
		require.NoError(t, model.DB.Create(&model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: "ag@example.com", Status: model.WtDonationActive}).Error)
		uploadsBefore = fake.requestsOf("POST /api/files")
		code, response = wtBuildDonate(t, session, "SID=secret-8")
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "You can donate at most 10 accounts", response.Message)
		assert.Equal(t, "You can donate at most {{max}} accounts", response.MessageKey)
		assert.Equal(t, uploadsBefore, fake.requestsOf("POST /api/files"))

		audits := wtAudits(t, "wt.donation_add")
		require.NotEmpty(t, audits)
		for _, audit := range audits {
			params := wtAuditParams(t, audit)
			assert.Contains(t, params, `"provider":"build"`)
			assert.NotContains(t, params, "secret", "audits carry no cookie values")
		}
		assert.Contains(t, wtAuditParams(t, audits[0]), `"success":false`)
	})
}

// The list of Build donations carries the console's view of every active
// account from one cached status; testing asks the console afresh; usage does
// not exist for this pool; removing deletes the auth file first and announces
// the removal of an active account.
func TestWtBuildListTestAndRemove(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeBuild(t)
		t.Setenv("WT_CPA_URL", "")
		t.Setenv("WT_CPA_MANAGEMENT_KEY", "")
		member := createWtUser(t, model.User{Username: "builder"})
		session := wtSession(t, member.Id)
		fake.hold(1, wtFakeBuildAccount{name: "wt-1-aaaa", hasContext: true})
		fake.hold(2, wtFakeBuildAccount{name: "wt-1-bbbb", expired: true})
		fake.hold(3, wtFakeBuildAccount{name: "wt-1-cccc", invalid: true})
		fake.hold(4, wtFakeBuildAccount{name: "wt-1-dddd"})
		fake.set(func(fake *wtFakeBuild) { fake.current = 1 })
		available := wtBuildDonation(t, member.Id, 1, 100)
		expired := wtBuildDonation(t, member.Id, 2, 200)
		invalid := wtBuildDonation(t, member.Id, 3, 300)
		idle := wtBuildDonation(t, member.Id, 4, 400)
		missing := wtBuildDonation(t, member.Id, 9, 500)
		failed := &model.WtDonation{UserId: member.Id, Provider: model.WtBuildProvider, Status: model.WtDonationFailed, FailReason: service.WtBuildFailGone, CredentialName: "7", CreatedAt: 600}
		require.NoError(t, model.DB.Create(failed).Error)

		type buildItem struct {
			ID         int                      `json:"id"`
			Provider   string                   `json:"provider"`
			Email      string                   `json:"email"`
			Status     string                   `json:"status"`
			FailReason string                   `json:"fail_reason"`
			Pool       *service.WtBuildPoolInfo `json:"pool"`
		}
		var list struct {
			Max   int         `json:"max"`
			Items []buildItem `json:"items"`
		}
		listBuild := func() {
			_, response := wtAisCall(t, session, WtListDonations, http.MethodGet, "/api/wt/donations?provider=build", "")
			require.True(t, response.Success, response.Message)
			require.NoError(t, common.Unmarshal(response.Data, &list))
		}
		listBuild()
		assert.Equal(t, 10, list.Max)
		require.Len(t, list.Items, 6)
		assert.Equal(t, failed.Id, list.Items[0].ID, "newest first: the failed row")
		assert.Equal(t, service.WtBuildFailGone, list.Items[0].FailReason)
		assert.Nil(t, list.Items[0].Pool)
		byID := map[int]buildItem{}
		for _, item := range list.Items {
			assert.Equal(t, model.WtBuildProvider, item.Provider)
			assert.Empty(t, item.Email)
			byID[item.ID] = item
		}
		for id, want := range map[int]service.WtBuildPoolInfo{
			available.Id: {State: service.WtBuildStateAvailable, Message: "Available", Index: 1, Current: true, HasContext: true},
			expired.Id:   {State: service.WtBuildStateExpired, Message: "The pool can no longer sign in with this account; remove it and donate again", Index: 2},
			invalid.Id:   {State: service.WtBuildStateInvalid, Message: "The pool could not read this account; remove it and donate again", Index: 3},
			idle.Id:      {State: service.WtBuildStateAvailable, Message: "Available", Index: 4},
			missing.Id:   {State: service.WtBuildStateMissing, Message: "The pool no longer has this account", Index: 9},
		} {
			require.NotNil(t, byID[id].Pool, id)
			assert.Equal(t, want, *byID[id].Pool, id)
		}
		listBuild()
		assert.Equal(t, 1, fake.requestsOf("GET /api/status"), "the status is cached for 15 seconds")

		service.WtResetDonationCaches()
		fake.set(func(fake *wtFakeBuild) { fake.down = true })
		listBuild()
		require.Len(t, list.Items, 6, "an unreachable console does not fail the page")
		for _, item := range list.Items {
			assert.Nil(t, item.Pool, "no state is shown while the console is down")
		}
		fake.set(func(fake *wtFakeBuild) { fake.down = false })

		var result struct {
			OK      bool             `json:"ok"`
			Message string           `json:"message"`
			Usage   *service.WtUsage `json:"usage"`
		}
		test := func(id int) securityEnrollmentResponse {
			_, response := wtAisCall(t, session, WtTestDonation, http.MethodPost, "/api/wt/donations/0/test", "", wtIDParam(id))
			return response
		}
		response := test(available.Id)
		require.True(t, response.Success, response.Message)
		require.NoError(t, common.Unmarshal(response.Data, &result))
		assert.True(t, result.OK)
		assert.Equal(t, "The pool can still use this account", result.Message)
		assert.Nil(t, result.Usage)
		assert.Equal(t, 1, wtDonationRow(t, available.Id).LastTestOk)
		response = test(expired.Id)
		require.True(t, response.Success, response.Message)
		require.NoError(t, common.Unmarshal(response.Data, &result))
		assert.False(t, result.OK)
		assert.Equal(t, "The pool can no longer sign in with this account", result.Message)
		assert.Equal(t, "Only an active donation can be tested", test(failed.Id).Message)
		audits := wtAudits(t, "wt.donation_test")
		require.Len(t, audits, 2)
		assert.Contains(t, wtAuditParams(t, audits[0]), `"provider":"build"`)

		code, response := wtAisCall(t, session, WtGetDonationUsage, http.MethodGet, "/api/wt/donations/0/usage", "", wtIDParam(available.Id))
		assert.Equal(t, http.StatusNotFound, code)
		assert.Equal(t, "Usage is not available for this pool", response.Message)

		remove := func(id int) securityEnrollmentResponse {
			_, response := wtAisCall(t, session, WtRemoveDonation, http.MethodPost, "/api/wt/donations/0/remove", "", wtIDParam(id))
			return response
		}
		response = remove(available.Id)
		require.True(t, response.Success, response.Message)
		assert.Equal(t, "Donation removed", response.Message)
		assert.Equal(t, []int{1}, fake.deletedSeen())
		assert.Equal(t, model.WtDonationRemoved, wtDonationRow(t, available.Id).Status)
		events := wtPoolEvents(t)
		require.Len(t, events, 1)
		assert.Equal(t, model.WtPoolEventBuildRemoved, events[0].Kind)
		assert.Equal(t, model.WtBuildProvider, events[0].Provider)
		assert.Equal(t, "builder", events[0].DisplayName)
		assert.Empty(t, events[0].Model)

		// A file the console no longer has is not an error; a failed donation
		// was never announced; a busy or silent console keeps the record.
		require.True(t, remove(missing.Id).Success)
		assert.Equal(t, model.WtDonationRemoved, wtDonationRow(t, missing.Id).Status)
		require.True(t, remove(failed.Id).Success)
		assert.Len(t, wtPoolEvents(t), 2)
		fake.set(func(fake *wtFakeBuild) { fake.busy = true })
		assert.Equal(t, "The pool is busy switching accounts; try again in a moment", remove(idle.Id).Message)
		assert.Equal(t, model.WtDonationActive, wtDonationRow(t, idle.Id).Status)
		fake.set(func(fake *wtFakeBuild) { fake.busy, fake.down = false, true })
		assert.Equal(t, "The pool is not answering; try again later", remove(idle.Id).Message)
		assert.Equal(t, model.WtDonationActive, wtDonationRow(t, idle.Id).Status)
		audits = wtAudits(t, "wt.donation_remove")
		require.Len(t, audits, 5)
		assert.Contains(t, wtAuditParams(t, audits[4]), `"success":false`)
	})
}

// The gate charges a "Build-" call against the BUILD ledger of the member:
// non-donors are refused, a donor has a hundred calls per account a day, an
// AIS PG donation does not open the BUILD pool, and the pool page shows the
// BUILD tank with its own give and take announcements.
func TestWtBuildGateAndPool(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		newWtFakeBuild(t)
		for _, name := range []string{"WT_CPA_URL", "WT_CPA_MANAGEMENT_KEY", "WT_AIS_URL", "WT_AIS_PASSWORD", "WT_AIS_API_KEY"} {
			t.Setenv(name, "")
		}
		var modelName atomic.Value
		modelName.Store(wtBuildFlashRelay)
		relay := gin.New()
		relay.POST("/v1/chat/completions", middleware.TokenAuth(), func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName.Load().(string))
		}, middleware.WtPoolGate(), func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})
		donor := createWtUser(t, model.User{Username: "builder", DisplayName: "Bea"})
		helper := createWtUser(t, model.User{Username: "helper", DisplayName: "Hal"})
		stranger := createWtUser(t, model.User{Username: "stranger"})
		donorKey := createWtAPIKey(t, donor.Id, common.TokenStatusEnabled)
		strangerKey := createWtAPIKey(t, stranger.Id, common.TokenStatusEnabled)
		wtBuildDonation(t, donor.Id, 0, 100)
		wtBuildDonation(t, helper.Id, 1, 200)
		wtAisDonation(t, stranger.Id, "acc-s", "s@example.com", 300)
		day := model.WtCheckinDay(wtDonationClock)
		ledgerOf := func(userID int, key string) model.WtAisLedger {
			var ledger model.WtAisLedger
			require.NoError(t, model.DB.Where("user_id = ? AND day = ? AND model = ?", userID, day, key).Limit(1).Find(&ledger).Error)
			return ledger
		}

		modelName.Store("Build-gemini-9")
		recorder := wtAisRelayCall(relay, donorKey)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "Unknown BUILD model")
		modelName.Store(wtBuildFlashRelay)

		recorder = wtAisRelayCall(relay, strangerKey)
		assert.Equal(t, http.StatusForbidden, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "Donate a BUILD account first to use the BUILD pool")
		assert.Zero(t, ledgerOf(stranger.Id, wtBuildFlashRelay).Id, "an AIS PG donor is not a BUILD donor")

		for i := range 100 {
			recorder = wtAisRelayCall(relay, donorKey)
			require.Equal(t, http.StatusNoContent, recorder.Code, "call %d", i+1)
			assert.Equal(t, strconv.Itoa(99-i), recorder.Header().Get("X-WT-AIS-Remaining"))
		}
		recorder = wtAisRelayCall(relay, donorKey)
		assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "Your BUILD calls for today are used up")
		assert.Equal(t, 100, ledgerOf(donor.Id, wtBuildFlashRelay).Calls)
		assert.Zero(t, ledgerOf(donor.Id, wtAisFlash).Id, "the BUILD ledger is not the AIS flash ledger")

		// The pool page: the BUILD tank beside an absent AIS tank.
		donorSession, helperSession, strangerSession := wtSession(t, donor.Id), wtSession(t, helper.Id), wtSession(t, stranger.Id)
		pool := wtReadPool(t, donorSession)
		assert.Nil(t, pool.Antigravity)
		assert.Nil(t, pool.Aistudio)
		require.NotNil(t, pool.Build)
		assert.Equal(t, model.WtBuildProvider, pool.Build.Provider)
		assert.Equal(t, day, pool.Build.Day)
		assert.Equal(t, 1, pool.Build.Accounts)
		require.Len(t, pool.Build.Models, 1)
		assert.Equal(t, model.WtAisModelBalance{Provider: model.WtBuildProvider, Model: wtBuildFlashRelay, RelayModel: wtBuildFlashRelay, PerAccount: 100, TakeMax: 100, Allowance: 100, Calls: 100, Remaining: 0, TakeLeft: 100}, pool.Build.Models[0])

		// Giving and taking move BUILD calls under their own event kinds; the
		// AIS PG model of the same upstream stays a different pool.
		var move struct {
			Model       string `json:"model"`
			Remaining   int    `json:"remaining"`
			PoolBalance int    `json:"pool_balance"`
			TakeLeft    int    `json:"take_left"`
		}
		give := func(member service.AuthIdentity, count int) securityEnrollmentResponse {
			_, response := wtAisCall(t, member, WtGiveAisCalls, http.MethodPost, "/api/wt/pool/ais/give", wtAisMoveBody(wtBuildFlashRelay, count))
			if response.Success {
				require.NoError(t, common.Unmarshal(response.Data, &move))
			}
			return response
		}
		take := func(member service.AuthIdentity, count int) securityEnrollmentResponse {
			_, response := wtAisCall(t, member, WtTakeAisCalls, http.MethodPost, "/api/wt/pool/ais/take", wtAisMoveBody(wtBuildFlashRelay, count))
			if response.Success {
				require.NoError(t, common.Unmarshal(response.Data, &move))
			}
			return response
		}
		assert.Equal(t, "Donate a BUILD account first to use the BUILD pool", give(strangerSession, 1).Message)
		_, response := wtAisCall(t, donorSession, WtGiveAisCalls, http.MethodPost, "/api/wt/pool/ais/give", wtAisMoveBody(wtAisFlash, 1))
		assert.Equal(t, "The AIS playground pool is not configured", response.Message)
		assert.Equal(t, "You can only give calls you still have today", give(donorSession, 1).Message)
		assert.Equal(t, "The pool does not hold that many calls right now", take(donorSession, 1).Message)
		require.True(t, give(helperSession, 30).Success)
		assert.Equal(t, wtBuildFlashRelay, move.Model)
		assert.Equal(t, 70, move.Remaining)
		assert.Equal(t, 30, move.PoolBalance)
		require.True(t, take(donorSession, 20).Success)
		assert.Equal(t, 20, move.Remaining)
		assert.Equal(t, 10, move.PoolBalance)
		assert.Equal(t, 80, move.TakeLeft)
		pool = wtReadPool(t, donorSession)
		assert.Equal(t, model.WtAisModelBalance{Provider: model.WtBuildProvider, Model: wtBuildFlashRelay, RelayModel: wtBuildFlashRelay, PerAccount: 100, TakeMax: 100, Allowance: 120, Calls: 100, Taken: 20, Remaining: 20, TakeLeft: 80, PoolBalance: 10, PoolGiven: 30, PoolTaken: 20}, pool.Build.Models[0])
		for i := range 20 {
			require.Equal(t, http.StatusNoContent, wtAisRelayCall(relay, donorKey).Code, "taken call %d", i+1)
		}
		assert.Equal(t, http.StatusTooManyRequests, wtAisRelayCall(relay, donorKey).Code)
		events := wtPoolEvents(t)
		require.Len(t, events, 2)
		assert.Equal(t, model.WtPoolEventBuildTaken, events[0].Kind)
		assert.Equal(t, model.WtBuildProvider, events[0].Provider)
		assert.Equal(t, "Bea", events[0].DisplayName)
		assert.Equal(t, 20, events[0].Amount)
		assert.Equal(t, wtBuildFlashRelay, events[0].Model)
		assert.Equal(t, model.WtPoolEventBuildGiven, events[1].Kind)
		assert.Equal(t, "Hal", events[1].DisplayName)
		assert.Equal(t, 30, events[1].Amount)
		gives := wtAudits(t, "wt.ais_give")
		require.NotEmpty(t, gives)
		assert.Contains(t, wtAuditParams(t, gives[len(gives)-1]), `"model":"Build-gemini-3.8-flash"`)
	})
}

// Every start puts the BUILD group, the ratio of its relay model and the
// wt-build channel back the way the site wants them, beside the AIS channel.
func TestWtEnsureBuildChannel(t *testing.T) {
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
		for _, name := range []string{"WT_BUILD_URL", "WT_BUILD_PASSWORD", "WT_BUILD_API_KEY"} {
			t.Setenv(name, "")
		}
		require.NoError(t, service.WtEnsureBuildChannel(), "an unconfigured pool is left alone")
		channels, err := model.GetChannelsByTag(service.WtBuildChannelTag, true, true)
		require.NoError(t, err)
		assert.Empty(t, channels)

		t.Setenv("WT_BUILD_URL", "http://aistudio-to-api:7860/")
		t.Setenv("WT_BUILD_PASSWORD", wtTestBuildPassword)
		t.Setenv("WT_BUILD_API_KEY", wtTestBuildKey)
		modelRatios := ratio_setting.GetModelRatioCopy()
		modelRatios[wtAisFlash] = 0.5
		encoded, err := common.Marshal(modelRatios)
		require.NoError(t, err)
		require.NoError(t, model.UpdateOption("ModelRatio", string(encoded)))
		for range 2 {
			require.NoError(t, service.WtEnsureBuildChannel())
		}
		assert.Equal(t, 1.0, ratio_setting.GetGroupRatioCopy()[service.WtBuildGroup])
		assert.Equal(t, "BUILD pool", setting.GetUserUsableGroupsCopy()[service.WtBuildGroup])
		assert.Equal(t, 0.5, ratio_setting.GetModelRatioCopy()[wtBuildFlashRelay], "the relay model bills like gemini-3.8-flash")
		assert.Equal(t, 1.0, ratio_setting.GetCompletionRatioCopy()[wtBuildFlashRelay])
		channels, err = model.GetChannelsByTag(service.WtBuildChannelTag, true, true)
		require.NoError(t, err)
		require.Len(t, channels, 1, "two starts create one channel")
		channel := channels[0]
		assert.Equal(t, constant.ChannelTypeGemini, channel.Type)
		assert.Equal(t, wtTestBuildKey, channel.Key)
		assert.Equal(t, "http://aistudio-to-api:7860", channel.GetBaseURL())
		assert.Equal(t, wtBuildFlashRelay, channel.Models)
		assert.Equal(t, "default,vip,svip,BUILD", channel.Group)
		var mapping map[string]string
		require.NoError(t, common.UnmarshalJsonStr(channel.GetModelMapping(), &mapping))
		assert.Equal(t, map[string]string{wtBuildFlashRelay: wtAisFlash}, mapping, "the relay model is served as gemini-3.8-flash")
		assert.Equal(t, service.WtBuildChannelName, channel.Name)
		assert.Equal(t, common.ChannelStatusEnabled, channel.Status)
		var abilities []model.Ability
		require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
		pairs := make([]string, 0, len(abilities))
		for _, ability := range abilities {
			assert.True(t, ability.Enabled)
			pairs = append(pairs, ability.Group+"|"+ability.Model)
		}
		assert.ElementsMatch(t, []string{"default|" + wtBuildFlashRelay, "vip|" + wtBuildFlashRelay, "svip|" + wtBuildFlashRelay, "BUILD|" + wtBuildFlashRelay}, pairs)

		// The AIS channel is a second channel, not a repair of this one.
		t.Setenv("WT_AIS_URL", "http://aistudio-api:8080")
		t.Setenv("WT_AIS_PASSWORD", wtTestAisPassword)
		t.Setenv("WT_AIS_API_KEY", wtTestAisKey)
		require.NoError(t, service.WtEnsureAisChannel())
		aisChannels, err := model.GetChannelsByTag(service.WtAisChannelTag, true, true)
		require.NoError(t, err)
		require.Len(t, aisChannels, 1)
		assert.NotEqual(t, channel.Id, aisChannels[0].Id)
		channels, err = model.GetChannelsByTag(service.WtBuildChannelTag, true, true)
		require.NoError(t, err)
		require.Len(t, channels, 1)
		assert.Equal(t, wtBuildFlashRelay, channels[0].Models)
	})
}
