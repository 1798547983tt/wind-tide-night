package controller

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wind-tide: regression tests of the WorkBuddy pool (wind-tide/API.md §16)
// against a scripted stand-in for the workbuddy2api panel: its sign-in start
// and poll, its status, its model list and the removal of an account.

const wtTestWbKey = "test-workbuddy-panel-key"

// wtFakeWbAccount is one account as the panel's status reports it.
type wtFakeWbAccount struct {
	uid, nickname, realm string
	credits, total       int64
	cooling              bool
	coolKind             string
	coolRemaining        int64
	disabled, paused     bool
	success, failed      int64
	checkinDone          bool
}

// wtFakeWbPoll is one scripted answer of the sign-in poll: the HTTP status
// (0 for 200) and the body.
type wtFakeWbPoll struct {
	status int
	body   map[string]any
}

// wtFakeWb plays the workbuddy2api panel for one test: a bearer key on every
// call, sign-ins that hand out a link and a state, polls answered from a
// script (the last answer repeats), a status that lists the accounts, the
// model list and removals.
type wtFakeWb struct {
	t      *testing.T
	server *httptest.Server
	mu     sync.Mutex

	down      bool
	noLink    bool
	starts    []string
	nextState int
	pending   []wtFakeWbPoll
	polls     map[string][]wtFakeWbPoll
	onPoll    func()
	accounts  map[string]*wtFakeWbAccount
	models    []string
	removed   []string
	requests  []string
	refused   int
}

func newWtFakeWb(t *testing.T) *wtFakeWb {
	t.Helper()
	fake := &wtFakeWb{t: t, polls: map[string][]wtFakeWbPoll{}, accounts: map[string]*wtFakeWbAccount{}}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	t.Setenv("WT_WB_URL", fake.server.URL)
	t.Setenv("WT_WB_API_KEY", wtTestWbKey)
	interval, attempts, inline := service.WtWbVerifyInterval, service.WtWbVerifyAttempts, service.WtDonationVerifySync
	service.WtWbVerifyInterval, service.WtWbVerifyAttempts, service.WtDonationVerifySync = time.Millisecond, 3, true
	t.Cleanup(func() {
		service.WtWbVerifyInterval, service.WtWbVerifyAttempts, service.WtDonationVerifySync = interval, attempts, inline
	})
	service.WtResetDonationCaches()
	t.Cleanup(service.WtResetDonationCaches)
	return fake
}

func (fake *wtFakeWb) handle(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	route := r.Method + " " + r.URL.Path
	fake.requests = append(fake.requests, route)
	if fake.down {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+wtTestWbKey {
		fake.refused++
		wtJSONReply(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid_api_key"})
		return
	}
	assert.Equal(fake.t, "application/json", r.Header.Get("Accept"))
	switch {
	case route == "GET /status":
		accounts := make([]map[string]any, 0, len(fake.accounts))
		var healthy, cooling, disabled int
		for _, uid := range slices.Sorted(maps.Keys(fake.accounts)) {
			account := fake.accounts[uid]
			accounts = append(accounts, map[string]any{
				"uid": account.uid, "nickname": account.nickname, "realm": account.realm,
				"credits": account.credits, "credits_total": account.total, "credits_expiring": 0,
				"cooling": account.cooling, "cool_kind": account.coolKind, "cool_remaining_sec": account.coolRemaining,
				"disabled": account.disabled, "disabled_reason": "", "paused": account.paused,
				"success_count": account.success, "err_total": account.failed, "checkin_done": account.checkinDone,
			})
			switch {
			case account.disabled:
				disabled++
			case account.cooling:
				cooling++
			default:
				healthy++
			}
		}
		wtJSONReply(w, http.StatusOK, map[string]any{"accounts": accounts, "total": len(accounts), "healthy": healthy, "cooling": cooling, "disabled": disabled, "redis_mode": "memory"})
	case route == "GET /v1/models":
		data := make([]map[string]any, 0, len(fake.models))
		for _, id := range fake.models {
			data = append(data, map[string]any{"id": id, "object": "model", "owned_by": "workbuddy"})
		}
		wtJSONReply(w, http.StatusOK, map[string]any{"object": "list", "data": data})
	case route == "POST /panel/api/login/start":
		var body map[string]string
		_ = common.DecodeJson(r.Body, &body)
		realm := body["realm"]
		if realm == "" {
			realm = "cn"
		}
		if fake.noLink {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("auth state (upstream 503): try again later"))
			return
		}
		// Only a sign-in Tencent handed a link for counts as started.
		fake.starts = append(fake.starts, realm)
		fake.nextState++
		state := fmt.Sprintf("state-%d", fake.nextState)
		fake.polls[state], fake.pending = fake.pending, nil
		wtJSONReply(w, http.StatusOK, map[string]any{"ok": true, "url": "https://login.example.test/oauth?state=" + state, "state": state, "realm": realm})
	case route == "GET /panel/api/login/poll":
		state := r.URL.Query().Get("state")
		if state == "" {
			wtJSONReply(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing state"})
			return
		}
		if fake.onPoll != nil {
			fake.onPoll()
			fake.onPoll = nil
		}
		answers := fake.polls[state]
		if len(answers) == 0 {
			wtJSONReply(w, http.StatusOK, map[string]any{"done": false, "message": "waiting for the sign-in"})
			return
		}
		answer := answers[0]
		if len(answers) > 1 {
			fake.polls[state] = answers[1:]
		}
		status := answer.status
		if status == 0 {
			status = http.StatusOK
		}
		if done, _ := answer.body["done"].(bool); done && status == http.StatusOK {
			uid, _ := answer.body["uid"].(string)
			nickname, _ := answer.body["nickname"].(string)
			realm, _ := answer.body["realm"].(string)
			if _, held := fake.accounts[uid]; !held && uid != "" {
				fake.accounts[uid] = &wtFakeWbAccount{uid: uid, nickname: nickname, realm: realm, credits: 500, total: 500}
			}
		}
		wtJSONReply(w, status, answer.body)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/panel/api/accounts/") && strings.HasSuffix(r.URL.Path, "/remove"):
		uid := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/panel/api/accounts/"), "/remove")
		fake.removed = append(fake.removed, uid)
		if fake.accounts[uid] == nil {
			wtJSONReply(w, http.StatusNotFound, map[string]any{"ok": false, "error": "account not found"})
			return
		}
		delete(fake.accounts, uid)
		wtJSONReply(w, http.StatusOK, map[string]any{"ok": true})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// set changes the fake under its lock.
func (fake *wtFakeWb) set(change func(fake *wtFakeWb)) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	change(fake)
}

// scriptNext sets the poll answers of the next sign-in the panel starts.
func (fake *wtFakeWb) scriptNext(answers ...wtFakeWbPoll) {
	fake.set(func(fake *wtFakeWb) { fake.pending = answers })
}

// script sets the poll answers of a sign-in the panel is already following.
func (fake *wtFakeWb) script(state string, answers ...wtFakeWbPoll) {
	fake.set(func(fake *wtFakeWb) { fake.polls[state] = answers })
}

// hold puts an account in the panel without a sign-in.
func (fake *wtFakeWb) hold(account wtFakeWbAccount) {
	fake.set(func(fake *wtFakeWb) {
		held := account
		fake.accounts[account.uid] = &held
	})
}

// requestsOf counts the calls of one route, "METHOD /path".
func (fake *wtFakeWb) requestsOf(route string) int {
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

func (fake *wtFakeWb) startsSeen() []string {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return slices.Clone(fake.starts)
}

func (fake *wtFakeWb) removedSeen() []string {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return slices.Clone(fake.removed)
}

// wtWbDone is the poll answer of a finished sign-in.
func wtWbDone(uid, nickname, realm string) wtFakeWbPoll {
	return wtFakeWbPoll{body: map[string]any{"done": true, "uid": uid, "nickname": nickname, "realm": realm, "credits": 500, "credits_total": 500, "checkin_message": "signed in"}}
}

// wtWbDonate starts a WorkBuddy donation for a member.
func wtWbDonate(t *testing.T, member service.AuthIdentity, body string) (int, securityEnrollmentResponse) {
	t.Helper()
	return wtAisCall(t, member, WtDonateWorkbuddy, http.MethodPost, "/api/wt/donations/workbuddy", body)
}

// wtWbDonation puts an active WorkBuddy donation of a panel account in the
// database, as a finished sign-in would.
func wtWbDonation(t *testing.T, userID int, uid, nickname string, createdAt int64) *model.WtDonation {
	t.Helper()
	donation := &model.WtDonation{
		UserId:         userID,
		Provider:       model.WtWbProvider,
		Status:         model.WtDonationActive,
		CredentialName: uid,
		Nickname:       nickname,
		CreatedAt:      createdAt,
		SubmittedAt:    createdAt,
		VerifiedAt:     createdAt,
	}
	require.NoError(t, model.DB.Create(donation).Error)
	return donation
}

// wtWbAccepted is the answer of a started donation.
type wtWbAccepted struct {
	ID               int    `json:"id"`
	Status           string `json:"status"`
	AuthorizationURL string `json:"authorization_url"`
	ExpiresAt        int64  `json:"expires_at"`
}

// Donating a WorkBuddy account: the consent and the realm are checked before
// the panel is asked, a panel without a link leaves no row, a finished
// sign-in binds the record to the account the panel added, a sign-in the
// panel forgot or nobody finishes fails, an account already in the pool is
// not taken twice, a record removed meanwhile gives its account back, and
// ten slots are the limit.
func TestWtWbDonateFlow(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeWb(t)
		for _, name := range []string{"WT_CPA_URL", "WT_CPA_MANAGEMENT_KEY", "WT_AIS_URL", "WT_AIS_PASSWORD", "WT_AIS_API_KEY", "WT_BUILD_URL", "WT_BUILD_PASSWORD", "WT_BUILD_API_KEY"} {
			t.Setenv(name, "")
		}
		member := createWtUser(t, model.User{Username: "buddy", DisplayName: "Bud"})
		session := wtSession(t, member.Id)

		flagsBody := decodeSecurityEnrollmentResponse(t, wtServe(GetStatus, http.MethodGet, "/api/status", "", 0))
		var flags map[string]any
		require.NoError(t, common.Unmarshal(flagsBody.Data, &flags))
		assert.Equal(t, true, flags["wt_wb_enabled"])
		assert.Equal(t, false, flags["wt_build_enabled"])

		// The consent and the realm are checked before the panel is asked.
		code, response := wtWbDonate(t, session, `{"realm":"cn"}`)
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Equal(t, "Accept the risks before donating an account", response.Message)
		code, response = wtWbDonate(t, session, `{"realm":`)
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Equal(t, "Invalid parameters", response.Message)
		code, response = wtWbDonate(t, session, `{"realm":"eu","accepted":true}`)
		assert.Equal(t, http.StatusBadRequest, code)
		assert.Equal(t, "Choose the CodeBuddy or the WorkBuddy sign-in", response.Message)
		assert.Zero(t, fake.requestsOf("POST /panel/api/login/start"), "nothing reached the panel")

		// A panel that does not answer or has no link to give leaves no row.
		fake.set(func(fake *wtFakeWb) { fake.down = true })
		code, response = wtWbDonate(t, session, `{"accepted":true}`)
		assert.Equal(t, http.StatusBadGateway, code)
		assert.Equal(t, "The pool is not answering; try again later", response.Message)
		fake.set(func(fake *wtFakeWb) { fake.down, fake.noLink = false, true })
		code, response = wtWbDonate(t, session, `{"accepted":true}`)
		assert.Equal(t, http.StatusBadGateway, code)
		assert.Equal(t, "WorkBuddy did not hand out a sign-in link; try again in a moment", response.Message)
		fake.set(func(fake *wtFakeWb) { fake.noLink = false })
		var count int64
		require.NoError(t, model.DB.Model(&model.WtDonation{}).Count(&count).Error)
		assert.Zero(t, count, "a refused start leaves no row")

		// Success: the member gets Tencent's link, the panel adds the account
		// when the sign-in finishes and the donation goes active with the
		// account's id and nickname; the realm defaults to CodeBuddy.
		fake.scriptNext(wtWbDone("1001", "Bud's CodeBuddy", "cn"))
		code, response = wtWbDonate(t, session, `{"accepted":true}`)
		require.Equal(t, http.StatusAccepted, code, response.Message)
		require.True(t, response.Success)
		var accepted wtWbAccepted
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		assert.Equal(t, model.WtDonationVerifying, accepted.Status)
		assert.True(t, strings.HasPrefix(accepted.AuthorizationURL, "https://login.example.test/oauth?state="), accepted.AuthorizationURL)
		assert.Equal(t, wtDonationClock.Add(15*time.Minute).Unix(), accepted.ExpiresAt)
		assert.Equal(t, []string{"cn"}, fake.startsSeen())
		row := wtDonationRow(t, accepted.ID)
		assert.Equal(t, model.WtWbProvider, row.Provider)
		assert.Equal(t, model.WtDonationActive, row.Status)
		assert.Equal(t, "1001", row.CredentialName, "the panel's account id")
		assert.Equal(t, "Bud's CodeBuddy", row.Nickname)
		assert.Empty(t, row.Email, "the panel reads no email")
		assert.Empty(t, row.CpaState, "the sign-in session is dropped once it is over")
		assert.Equal(t, wtDonationClock.Unix(), row.VerifiedAt)
		events := wtPoolEvents(t)
		require.Len(t, events, 1)
		assert.Equal(t, model.WtPoolEventWbDonated, events[0].Kind)
		assert.Equal(t, model.WtWbProvider, events[0].Provider)
		assert.Equal(t, "Bud", events[0].DisplayName)
		assert.Equal(t, 1, events[0].Amount)
		assert.Empty(t, events[0].Model, "account events carry no model")

		// The single row shows the nickname and never the sign-in session.
		_, response = wtAisCall(t, session, WtGetDonation, http.MethodGet, "/api/wt/donations/0", "", wtIDParam(accepted.ID))
		require.True(t, response.Success, response.Message)
		var single map[string]any
		require.NoError(t, common.Unmarshal(response.Data, &single))
		assert.Equal(t, "Bud's CodeBuddy", single["nickname"])
		assert.Equal(t, model.WtDonationActive, single["status"])
		assert.NotContains(t, string(response.Data), "state-")

		// The WorkBuddy realm is passed on; a sign-in the panel forgot fails
		// as not finished in time.
		fake.scriptNext(wtFakeWbPoll{status: http.StatusNotFound, body: map[string]any{"ok": false, "error": "unknown or expired state"}})
		_, response = wtWbDonate(t, session, `{"realm":"global","accepted":true}`)
		require.True(t, response.Success, response.Message)
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		row = wtDonationRow(t, accepted.ID)
		assert.Equal(t, model.WtDonationFailed, row.Status)
		assert.Equal(t, service.WtWbFailTimeout, row.FailReason)
		assert.Equal(t, []string{"cn", "global"}, fake.startsSeen())

		// A panel that stumbles is asked again; a sign-in nobody finishes
		// fails once the attempts are over.
		fake.scriptNext(wtFakeWbPoll{status: http.StatusBadGateway, body: map[string]any{"ok": false, "error": "token issued but no uid"}}, wtFakeWbPoll{body: map[string]any{"done": false}})
		polls := fake.requestsOf("GET /panel/api/login/poll")
		_, response = wtWbDonate(t, session, `{"accepted":true}`)
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		assert.Equal(t, service.WtWbFailTimeout, wtDonationRow(t, accepted.ID).FailReason)
		assert.Equal(t, polls+service.WtWbVerifyAttempts, fake.requestsOf("GET /panel/api/login/poll"))

		// The same account signed in again, by anyone, is not taken twice:
		// the first donation keeps it and the panel keeps the account.
		other := createWtUser(t, model.User{Username: "other"})
		fake.scriptNext(wtWbDone("1001", "Bud's CodeBuddy", "cn"))
		_, response = wtWbDonate(t, wtSession(t, other.Id), `{"accepted":true}`)
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		row = wtDonationRow(t, accepted.ID)
		assert.Equal(t, model.WtDonationFailed, row.Status)
		assert.Equal(t, service.WtWbFailDuplicate, row.FailReason)
		assert.Empty(t, row.CredentialName)
		assert.Equal(t, model.WtDonationActive, wtDonationRow(t, 1).Status)
		assert.Empty(t, fake.removedSeen())
		assert.Len(t, wtPoolEvents(t), 1, "failed donations are not announced")

		// A record the member removed while signing in gives the account
		// back to Tencent: the panel is asked to drop it.
		fake.set(func(fake *wtFakeWb) {
			fake.onPoll = func() {
				require.NoError(t, model.DB.Model(&model.WtDonation{}).Where("provider = ? AND status = ?", model.WtWbProvider, model.WtDonationVerifying).Update("status", model.WtDonationRemoved).Error)
			}
		})
		fake.scriptNext(wtWbDone("2002", "Gone", "cn"))
		_, response = wtWbDonate(t, session, `{"accepted":true}`)
		require.NoError(t, common.Unmarshal(response.Data, &accepted))
		assert.Equal(t, model.WtDonationRemoved, wtDonationRow(t, accepted.ID).Status)
		assert.Equal(t, []string{"2002"}, fake.removedSeen())
		assert.Len(t, wtPoolEvents(t), 1)

		// Ten verifying or active WorkBuddy donations fill the slots; the
		// failed ones and other pools' donations do not count.
		for i := range 9 {
			require.NoError(t, model.DB.Create(&model.WtDonation{UserId: member.Id, Provider: model.WtWbProvider, Status: model.WtDonationVerifying, CpaState: fmt.Sprintf("slot-%d", i)}).Error)
		}
		require.NoError(t, model.DB.Create(&model.WtDonation{UserId: member.Id, Provider: "antigravity", Email: "ag@example.com", Status: model.WtDonationActive}).Error)
		starts := fake.requestsOf("POST /panel/api/login/start")
		code, response = wtWbDonate(t, session, `{"accepted":true}`)
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "You can donate at most 10 accounts", response.Message)
		assert.Equal(t, "You can donate at most {{max}} accounts", response.MessageKey)
		assert.Equal(t, starts, fake.requestsOf("POST /panel/api/login/start"))

		audits := wtAudits(t, "wt.donation_add")
		require.Len(t, audits, 7, "two refused starts and five sign-ins")
		for _, audit := range audits {
			params := wtAuditParams(t, audit)
			assert.Contains(t, params, `"provider":"workbuddy"`)
			assert.NotContains(t, params, "state-", "audits carry no sign-in session")
		}
		assert.Contains(t, wtAuditParams(t, audits[0]), `"success":false`)
		assert.Contains(t, wtAuditParams(t, audits[2]), `"realm":"cn"`)
		assert.Contains(t, wtAuditParams(t, audits[2]), `"success":true`)
		assert.Contains(t, wtAuditParams(t, audits[3]), `"realm":"global"`)
	})
}

// The list of WorkBuddy donations carries the panel's view of every active
// account from one cached status; testing asks the panel afresh; usage does
// not exist for this pool; removing takes the account out of the panel first
// and announces the removal of an active account.
func TestWtWbListTestAndRemove(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeWb(t)
		t.Setenv("WT_CPA_URL", "")
		t.Setenv("WT_CPA_MANAGEMENT_KEY", "")
		member := createWtUser(t, model.User{Username: "buddy"})
		session := wtSession(t, member.Id)
		fake.hold(wtFakeWbAccount{uid: "a1", nickname: "Alpha", realm: "cn", credits: 320, total: 500, success: 12, failed: 1, checkinDone: true})
		fake.hold(wtFakeWbAccount{uid: "a2", nickname: "Soft", realm: "cn", credits: 200, total: 500, cooling: true, coolKind: "soft_rate", coolRemaining: 60})
		fake.hold(wtFakeWbAccount{uid: "a3", nickname: "Hard", realm: "global", total: 500, cooling: true, coolKind: "hard_credit", coolRemaining: 3600})
		fake.hold(wtFakeWbAccount{uid: "a4", nickname: "Paused", realm: "cn", credits: 500, total: 500, paused: true})
		fake.hold(wtFakeWbAccount{uid: "a5", nickname: "Dead", realm: "cn", credits: 500, total: 500, disabled: true})
		available := wtWbDonation(t, member.Id, "a1", "Alpha", 100)
		soft := wtWbDonation(t, member.Id, "a2", "Soft", 200)
		hard := wtWbDonation(t, member.Id, "a3", "Hard", 300)
		paused := wtWbDonation(t, member.Id, "a4", "Paused", 400)
		disabled := wtWbDonation(t, member.Id, "a5", "Dead", 500)
		missing := wtWbDonation(t, member.Id, "a9", "Lost", 600)
		failed := &model.WtDonation{UserId: member.Id, Provider: model.WtWbProvider, Status: model.WtDonationFailed, FailReason: service.WtWbFailTimeout, CreatedAt: 700}
		require.NoError(t, model.DB.Create(failed).Error)

		type wbItem struct {
			ID         int                   `json:"id"`
			Provider   string                `json:"provider"`
			Email      string                `json:"email"`
			Nickname   string                `json:"nickname"`
			Status     string                `json:"status"`
			FailReason string                `json:"fail_reason"`
			Pool       *service.WtWbPoolInfo `json:"pool"`
		}
		var list struct {
			Max   int      `json:"max"`
			Items []wbItem `json:"items"`
		}
		listWb := func() {
			_, response := wtAisCall(t, session, WtListDonations, http.MethodGet, "/api/wt/donations?provider=workbuddy", "")
			require.True(t, response.Success, response.Message)
			require.NoError(t, common.Unmarshal(response.Data, &list))
		}
		listWb()
		assert.Equal(t, 10, list.Max)
		require.Len(t, list.Items, 7)
		assert.Equal(t, failed.Id, list.Items[0].ID, "newest first: the failed row")
		assert.Equal(t, service.WtWbFailTimeout, list.Items[0].FailReason)
		assert.Nil(t, list.Items[0].Pool)
		byID := map[int]wbItem{}
		for _, item := range list.Items {
			assert.Equal(t, model.WtWbProvider, item.Provider)
			assert.Empty(t, item.Email)
			byID[item.ID] = item
		}
		assert.Equal(t, "Alpha", byID[available.Id].Nickname)
		assert.Equal(t, "Lost", byID[missing.Id].Nickname)
		for id, want := range map[int]service.WtWbPoolInfo{
			available.Id: {State: service.WtWbStateAvailable, Message: "Available", Nickname: "Alpha", Realm: "cn", Credits: 320, CreditsTotal: 500, Success: 12, Failed: 1, CheckedIn: true},
			soft.Id:      {State: service.WtWbStateCooldown, Message: "Rate-limited; the pool is resting this account", Nickname: "Soft", Realm: "cn", Credits: 200, CreditsTotal: 500, RetryAt: wtDonationClock.Add(time.Minute).Unix()},
			hard.Id:      {State: service.WtWbStateCooldown, Message: "Credits used up for today", Nickname: "Hard", Realm: "global", CreditsTotal: 500, RetryAt: wtDonationClock.Add(time.Hour).Unix()},
			paused.Id:    {State: service.WtWbStatePaused, Message: "The pool owner paused this account", Nickname: "Paused", Realm: "cn", Credits: 500, CreditsTotal: 500},
			disabled.Id:  {State: service.WtWbStateDisabled, Message: "The pool can no longer sign in with this account; remove it and donate again", Nickname: "Dead", Realm: "cn", Credits: 500, CreditsTotal: 500},
			missing.Id:   {State: service.WtWbStateMissing, Message: "The pool no longer has this account"},
		} {
			require.NotNil(t, byID[id].Pool, id)
			assert.Equal(t, want, *byID[id].Pool, id)
		}
		listWb()
		assert.Equal(t, 1, fake.requestsOf("GET /status"), "the status is cached for 15 seconds")

		service.WtResetDonationCaches()
		fake.set(func(fake *wtFakeWb) { fake.down = true })
		listWb()
		require.Len(t, list.Items, 7, "an unreachable panel does not fail the page")
		for _, item := range list.Items {
			assert.Nil(t, item.Pool, "no state is shown while the panel is down")
		}
		fake.set(func(fake *wtFakeWb) { fake.down = false })

		// A refused key is the pool not answering, never a crash.
		t.Setenv("WT_WB_API_KEY", "wrong-key")
		service.WtResetDonationCaches()
		listWb()
		for _, item := range list.Items {
			assert.Nil(t, item.Pool, "a refused key shows no state")
		}
		t.Setenv("WT_WB_API_KEY", wtTestWbKey)

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
		response = test(paused.Id)
		require.NoError(t, common.Unmarshal(response.Data, &result))
		assert.True(t, result.OK, "a paused account is still signed in")
		response = test(disabled.Id)
		require.True(t, response.Success, response.Message)
		require.NoError(t, common.Unmarshal(response.Data, &result))
		assert.False(t, result.OK)
		assert.Equal(t, "The pool can no longer use this account", result.Message)
		response = test(missing.Id)
		require.NoError(t, common.Unmarshal(response.Data, &result))
		assert.False(t, result.OK)
		assert.Equal(t, "Only an active donation can be tested", test(failed.Id).Message)
		audits := wtAudits(t, "wt.donation_test")
		require.Len(t, audits, 4)
		assert.Contains(t, wtAuditParams(t, audits[0]), `"provider":"workbuddy"`)

		code, response := wtAisCall(t, session, WtGetDonationUsage, http.MethodGet, "/api/wt/donations/0/usage", "", wtIDParam(available.Id))
		assert.Equal(t, http.StatusNotFound, code)
		assert.Equal(t, "Usage is not available for this pool", response.Message)

		remove := func(id int) securityEnrollmentResponse {
			_, response := wtAisCall(t, session, WtRemoveDonation, http.MethodPost, "/api/wt/donations/0/remove", "", wtIDParam(id))
			return response
		}
		statusReads := fake.requestsOf("GET /status")
		response = remove(available.Id)
		require.True(t, response.Success, response.Message)
		assert.Equal(t, "Donation removed", response.Message)
		assert.Equal(t, []string{"a1"}, fake.removedSeen())
		assert.Equal(t, model.WtDonationRemoved, wtDonationRow(t, available.Id).Status)
		events := wtPoolEvents(t)
		require.Len(t, events, 1)
		assert.Equal(t, model.WtPoolEventWbRemoved, events[0].Kind)
		assert.Equal(t, model.WtWbProvider, events[0].Provider)
		assert.Equal(t, "buddy", events[0].DisplayName)
		assert.Empty(t, events[0].Model)
		listWb()
		assert.Equal(t, statusReads+1, fake.requestsOf("GET /status"), "the removal drops the cached status")
		assert.Len(t, list.Items, 6)

		// An account the panel no longer has is not an error, and the record was
		// active, so its removal is announced; a failed donation was never
		// announced; a silent panel keeps the record.

		require.True(t, remove(missing.Id).Success)
		assert.Equal(t, model.WtDonationRemoved, wtDonationRow(t, missing.Id).Status)
		require.True(t, remove(failed.Id).Success)
		assert.Len(t, wtPoolEvents(t), 2)
		fake.set(func(fake *wtFakeWb) { fake.down = true })
		assert.Equal(t, "The pool is not answering; try again later", remove(soft.Id).Message)
		assert.Equal(t, model.WtDonationActive, wtDonationRow(t, soft.Id).Status)
		audits = wtAudits(t, "wt.donation_remove")
		require.Len(t, audits, 4)
		assert.Contains(t, wtAuditParams(t, audits[3]), `"success":false`)
	})
}

// The pool page shows the WorkBuddy tank from the panel's cached status, a
// tank without a reading while the panel is down and no tank while the pool
// is not configured; a restart follows the sign-ins it left verifying.
func TestWtWbPoolPageAndRestart(t *testing.T) {
	forEachWtDatabase(t, func(t *testing.T) {
		pinWtClock(t, wtDonationClock)
		fake := newWtFakeWb(t)
		for _, name := range []string{"WT_CPA_URL", "WT_CPA_MANAGEMENT_KEY", "WT_AIS_URL", "WT_AIS_PASSWORD", "WT_AIS_API_KEY", "WT_BUILD_URL", "WT_BUILD_PASSWORD", "WT_BUILD_API_KEY"} {
			t.Setenv(name, "")
		}
		member := createWtUser(t, model.User{Username: "buddy", DisplayName: "Bud"})
		session := wtSession(t, member.Id)
		fake.hold(wtFakeWbAccount{uid: "a1", nickname: "Alpha", realm: "cn", credits: 300, total: 500})
		fake.hold(wtFakeWbAccount{uid: "a2", nickname: "Soft", realm: "cn", credits: 100, total: 500, cooling: true, coolKind: "soft_rate", coolRemaining: 30})
		fake.hold(wtFakeWbAccount{uid: "a3", nickname: "Dead", realm: "cn", credits: 50, total: 500, disabled: true})

		reply := wtReadPool(t, session)
		require.NotNil(t, reply.Workbuddy)
		assert.Equal(t, 3, reply.Workbuddy.Accounts)
		assert.Equal(t, 1, reply.Workbuddy.Healthy)
		assert.Equal(t, 1, reply.Workbuddy.Cooling)
		assert.EqualValues(t, 400, reply.Workbuddy.Credits, "a disabled account's credits are not counted")
		assert.EqualValues(t, 1000, reply.Workbuddy.CreditsTotal)
		assert.NotZero(t, reply.Workbuddy.FetchedAt)
		assert.Nil(t, reply.Antigravity)
		assert.Nil(t, reply.Aistudio)
		assert.Nil(t, reply.Build)

		service.WtResetDonationCaches()
		fake.set(func(fake *wtFakeWb) { fake.down = true })
		reply = wtReadPool(t, session)
		require.NotNil(t, reply.Workbuddy, "the tank stays while the panel is down")
		assert.Zero(t, reply.Workbuddy.FetchedAt)
		assert.Zero(t, reply.Workbuddy.Accounts)
		fake.set(func(fake *wtFakeWb) { fake.down = false })

		// A restart follows the sign-ins it left verifying; one without a
		// session fails.
		fake.script("state-7", wtWbDone("7001", "Seven", "cn"))
		followed := &model.WtDonation{UserId: member.Id, Provider: model.WtWbProvider, Status: model.WtDonationVerifying, CpaState: "state-7", CreatedAt: 100}
		orphan := &model.WtDonation{UserId: member.Id, Provider: model.WtWbProvider, Status: model.WtDonationVerifying, CreatedAt: 200}
		require.NoError(t, model.DB.Create(followed).Error)
		require.NoError(t, model.DB.Create(orphan).Error)
		service.WtSettleVerifyingWbDonations()
		row := wtDonationRow(t, followed.Id)
		assert.Equal(t, model.WtDonationActive, row.Status)
		assert.Equal(t, "7001", row.CredentialName)
		assert.Equal(t, "Seven", row.Nickname)
		row = wtDonationRow(t, orphan.Id)
		assert.Equal(t, model.WtDonationFailed, row.Status)
		assert.Equal(t, service.WtWbFailTimeout, row.FailReason)
		events := wtPoolEvents(t)
		require.Len(t, events, 1)
		assert.Equal(t, model.WtPoolEventWbDonated, events[0].Kind)
		assert.Equal(t, "Bud", events[0].DisplayName)

		// Without the panel's variables the pool is not configured.
		t.Setenv("WT_WB_URL", "")
		flagsBody := decodeSecurityEnrollmentResponse(t, wtServe(GetStatus, http.MethodGet, "/api/status", "", 0))
		var flags map[string]any
		require.NoError(t, common.Unmarshal(flagsBody.Data, &flags))
		assert.Equal(t, false, flags["wt_wb_enabled"])
		_, response := wtAisCall(t, session, WtListDonations, http.MethodGet, "/api/wt/donations?provider=workbuddy", "")
		assert.Equal(t, "The WorkBuddy pool is not configured", response.Message)
		_, response = wtWbDonate(t, session, `{"accepted":true}`)
		assert.Equal(t, "The WorkBuddy pool is not configured", response.Message)
	})
}

// The WorkBuddy channel is created once from the panel's model list under
// WB- names, with a ratio for each; later starts repair the connection only
// and leave the owner's models, groups and status alone; a channel born with
// the seed list takes the panel's list once the panel has one.
func TestWtEnsureWbChannel(t *testing.T) {
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
		fake := newWtFakeWb(t)
		t.Setenv("WT_WB_URL", "")
		t.Setenv("WT_WB_API_KEY", "")
		require.NoError(t, service.WtEnsureWbChannel(), "an unconfigured pool is left alone")
		channels, err := model.GetChannelsByTag(service.WtWbChannelTag, true, true)
		require.NoError(t, err)
		assert.Empty(t, channels)

		t.Setenv("WT_WB_URL", fake.server.URL+"/")
		t.Setenv("WT_WB_API_KEY", wtTestWbKey)
		modelRatios := ratio_setting.GetModelRatioCopy()
		modelRatios["glm-5.2"] = 0.5
		encoded, err := common.Marshal(modelRatios)
		require.NoError(t, err)
		require.NoError(t, model.UpdateOption("ModelRatio", string(encoded)))
		fake.set(func(fake *wtFakeWb) {
			fake.models = []string{"cn:glm-5.2", "global:gpt-5.4", "cn:glm-5.2", " ", "bad,name"}
		})
		for range 2 {
			require.NoError(t, service.WtEnsureWbChannel())
		}
		channels, err = model.GetChannelsByTag(service.WtWbChannelTag, true, true)
		require.NoError(t, err)
		require.Len(t, channels, 1, "two starts create one channel")
		channel := channels[0]
		assert.Equal(t, constant.ChannelTypeOpenAI, channel.Type)
		assert.Equal(t, wtTestWbKey, channel.Key)
		assert.Equal(t, fake.server.URL, channel.GetBaseURL())
		assert.Equal(t, "WB-glm-5.2,WB-global-gpt-5.4", channel.Models, "duplicates and bad ids are dropped")
		assert.Equal(t, "default,vip,svip", channel.Group, "no gate, no group of its own")
		var mapping map[string]string
		require.NoError(t, common.UnmarshalJsonStr(channel.GetModelMapping(), &mapping))
		assert.Equal(t, map[string]string{"WB-glm-5.2": "cn:glm-5.2", "WB-global-gpt-5.4": "global:gpt-5.4"}, mapping)
		assert.Equal(t, service.WtWbChannelName, channel.Name)
		assert.Equal(t, common.ChannelStatusEnabled, channel.Status)
		assert.Zero(t, channel.GetPriority())
		assert.Zero(t, channel.GetWeight())
		assert.Equal(t, 0.5, ratio_setting.GetModelRatioCopy()["WB-glm-5.2"], "the relay model bills like glm-5.2")
		wantGPT := 1.0
		if upstream, ok := ratio_setting.GetModelRatioCopy()["gpt-5.4"]; ok {
			wantGPT = upstream
		}
		assert.Equal(t, wantGPT, ratio_setting.GetModelRatioCopy()["WB-global-gpt-5.4"])
		wantCompletion := 1.0
		if upstream, ok := ratio_setting.GetCompletionRatioCopy()["glm-5.2"]; ok {
			wantCompletion = upstream
		}
		assert.Equal(t, wantCompletion, ratio_setting.GetCompletionRatioCopy()["WB-glm-5.2"])
		var abilities []model.Ability
		require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
		pairs := make([]string, 0, len(abilities))
		for _, ability := range abilities {
			assert.True(t, ability.Enabled)
			pairs = append(pairs, ability.Group+"|"+ability.Model)
		}
		assert.ElementsMatch(t, []string{"default|WB-glm-5.2", "vip|WB-glm-5.2", "svip|WB-glm-5.2", "default|WB-global-gpt-5.4", "vip|WB-global-gpt-5.4", "svip|WB-global-gpt-5.4"}, pairs)

		// The owner's models, groups, mapping and status stay; a changed key
		// or address is repaired; a ratio the owner set stays too.
		ownerMapping := `{"WB-glm-5.2":"cn:glm-5.2"}`
		channel.Models = "WB-glm-5.2"
		channel.ModelMapping = &ownerMapping
		channel.Group = "vip,svip"
		channel.Status = common.ChannelStatusManuallyDisabled
		require.NoError(t, channel.Update())
		modelRatios = ratio_setting.GetModelRatioCopy()
		modelRatios["WB-glm-5.2"] = 2.5
		encoded, err = common.Marshal(modelRatios)
		require.NoError(t, err)
		require.NoError(t, model.UpdateOption("ModelRatio", string(encoded)))
		t.Setenv("WT_WB_API_KEY", "rotated-key")
		fake.set(func(fake *wtFakeWb) { fake.models = []string{"cn:glm-5.2", "cn:kimi-k3"} })
		require.NoError(t, service.WtEnsureWbChannel())
		channels, err = model.GetChannelsByTag(service.WtWbChannelTag, true, true)
		require.NoError(t, err)
		require.Len(t, channels, 1)
		assert.Equal(t, channel.Id, channels[0].Id)
		assert.Equal(t, "rotated-key", channels[0].Key, "the key is repaired")
		assert.Equal(t, "WB-glm-5.2", channels[0].Models, "the owner's list is kept")
		assert.Equal(t, "vip,svip", channels[0].Group)
		assert.Equal(t, common.ChannelStatusManuallyDisabled, channels[0].Status)
		assert.JSONEq(t, ownerMapping, channels[0].GetModelMapping())
		assert.Equal(t, 2.5, ratio_setting.GetModelRatioCopy()["WB-glm-5.2"])
		t.Setenv("WT_WB_API_KEY", wtTestWbKey)

		// A channel born while the panel served nothing carries the seed
		// list; the panel's list replaces it once, and is then left alone.
		require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).Delete(&model.Ability{}).Error)
		require.NoError(t, model.DB.Delete(&model.Channel{}, channel.Id).Error)
		model.InitChannelCache()
		fake.set(func(fake *wtFakeWb) { fake.models = nil })
		require.NoError(t, service.WtEnsureWbChannel())
		channels, err = model.GetChannelsByTag(service.WtWbChannelTag, true, true)
		require.NoError(t, err)
		require.Len(t, channels, 1)
		seeded := strings.Split(channels[0].Models, ",")
		assert.Greater(t, len(seeded), 10, "the seed list is the panel's catalog")
		for _, name := range seeded {
			assert.True(t, strings.HasPrefix(name, service.WtWbModelPrefix), name)
		}
		assert.Contains(t, seeded, "WB-glm-5.2")
		assert.Contains(t, seeded, "WB-global-gpt-5.4")
		var seedMapping map[string]string
		require.NoError(t, common.UnmarshalJsonStr(channels[0].GetModelMapping(), &seedMapping))
		assert.Equal(t, "cn:glm-5.2", seedMapping["WB-glm-5.2"])
		assert.Equal(t, "global:gpt-5.4", seedMapping["WB-global-gpt-5.4"])
		assert.Len(t, seedMapping, len(seeded))
		wantKimi := 1.0
		if upstream, ok := ratio_setting.GetModelRatioCopy()["kimi-k2.5"]; ok {
			wantKimi = upstream
		}
		assert.Equal(t, wantKimi, ratio_setting.GetModelRatioCopy()["WB-global-kimi-k2.5"], "a seeded model bills like its upstream, or at 1")

		fake.set(func(fake *wtFakeWb) { fake.models = []string{"cn:glm-5.3"} })
		require.NoError(t, service.WtEnsureWbChannel())
		channels, err = model.GetChannelsByTag(service.WtWbChannelTag, true, true)
		require.NoError(t, err)
		require.Len(t, channels, 1)
		assert.Equal(t, "WB-glm-5.3", channels[0].Models, "the panel's first list replaces the seed")
		var replaced map[string]string
		require.NoError(t, common.UnmarshalJsonStr(channels[0].GetModelMapping(), &replaced))
		assert.Equal(t, map[string]string{"WB-glm-5.3": "cn:glm-5.3"}, replaced)
		fake.set(func(fake *wtFakeWb) { fake.models = []string{"cn:glm-5.3", "cn:kimi-k3"} })
		require.NoError(t, service.WtEnsureWbChannel())
		channels, err = model.GetChannelsByTag(service.WtWbChannelTag, true, true)
		require.NoError(t, err)
		assert.Equal(t, "WB-glm-5.3", channels[0].Models, "a list that is no longer the seed is the owner's")
	})
}
