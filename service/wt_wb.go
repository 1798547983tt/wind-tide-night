package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// wind-tide: client of the workbuddy2api panel that holds the donated
// WorkBuddy (Tencent CodeBuddy) accounts (wind-tide/API.md §16). The panel
// takes one API key, as a bearer token, for its relay, its status and its
// management calls; the key never leaves this process, every path is fixed,
// redirects are not followed and responses are capped. The accounts' tokens
// stay in the panel: a member signs in on Tencent's page, the panel collects
// the token, and the site learns the account id and the nickname only. Log
// lines carry the operation and the HTTP status only.

const (
	WtWbChannelName = "WorkBuddy"
	WtWbChannelTag  = "wt-wb"

	// WtWbRealmCN and WtWbRealmGlobal are the two sign-ins the panel knows:
	// CodeBuddy in China and WorkBuddy elsewhere.
	WtWbRealmCN     = "cn"
	WtWbRealmGlobal = "global"

	// WtWbSignInTTL is how long the panel keeps an unfinished sign-in; the
	// verifier gives up at the same time.
	WtWbSignInTTL = 15 * time.Minute

	wtWbTimeout        = 20 * time.Second
	wtWbMaxBody        = 1 << 20
	wtWbStatusCacheTTL = 15 * time.Second
	wtWbUIDMax         = 64
	// wtWbCoolHard is the panel's name for an account that spent its credits
	// for the day; the other kind is a rate limit.
	wtWbCoolHard = "hard_credit"
	// wtWbStateMax is what the donation record can hold of a sign-in session.
	wtWbStateMax = 128
)

// The pool states of an active WorkBuddy donation (wind-tide/API.md §16).
const (
	WtWbStateAvailable = "available"
	WtWbStateCooldown  = "cooldown"
	WtWbStatePaused    = "paused"
	WtWbStateDisabled  = "disabled"
	WtWbStateMissing   = "missing"
)

var (
	// ErrWtWbUnavailable stands for every failure to talk to the panel.
	ErrWtWbUnavailable = common.NewMessage("The pool is not answering; try again later")
	// ErrWtWbNotConfigured is the answer while the WorkBuddy variables are not set.
	ErrWtWbNotConfigured = common.NewMessage("The WorkBuddy pool is not configured")
	// ErrWtWbSignInLink is Tencent not handing the panel a sign-in link.
	ErrWtWbSignInLink = common.NewMessage("WorkBuddy did not hand out a sign-in link; try again in a moment")
	// ErrWtWbSignInExpired is the panel no longer knowing the sign-in.
	ErrWtWbSignInExpired = common.NewMessage("This sign-in has expired, start again")
)

var (
	wtWbStatusMu sync.Mutex
	wtWbStatus   *WtWbStatus
)

// WtWbEnabled reports whether the site is wired to a WorkBuddy pool: the
// panel address and its API key are both set.
func WtWbEnabled() bool {
	return wtWbURL() != "" && WtWbAPIKey() != ""
}

func wtWbURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("WT_WB_URL")), "/")
}

// WtWbAPIKey is the panel's one key; it becomes the key of the site's
// WorkBuddy channel too.
func WtWbAPIKey() string {
	return strings.TrimSpace(os.Getenv("WT_WB_API_KEY"))
}

// WtWbValidRealm reports whether realm is one of the two sign-ins.
func WtWbValidRealm(realm string) bool {
	return realm == WtWbRealmCN || realm == WtWbRealmGlobal
}

// wtWbValidUID is the panel's own rule for an account id: letters, digits,
// underscores and hyphens, at most 64 of them. An id that fails it never
// becomes part of a path.
func wtWbValidUID(uid string) bool {
	if uid == "" || len(uid) > wtWbUIDMax {
		return false
	}
	for _, c := range uid {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// WtWbAccount is what the panel reports about one account, without secrets.
type WtWbAccount struct {
	UID            string `json:"uid"`
	Nickname       string `json:"nickname"`
	Realm          string `json:"realm"`
	Credits        int64  `json:"credits"`
	CreditsTotal   int64  `json:"credits_total"`
	Cooling        bool   `json:"cooling"`
	CoolKind       string `json:"cool_kind"`
	CoolRemaining  int64  `json:"cool_remaining_sec"`
	Disabled       bool   `json:"disabled"`
	DisabledReason string `json:"disabled_reason"`
	Paused         bool   `json:"paused"`
	SuccessCount   int64  `json:"success_count"`
	ErrTotal       int64  `json:"err_total"`
	CheckinDone    bool   `json:"checkin_done"`
}

// WtWbStatus is the part of the panel's status the site reads.
type WtWbStatus struct {
	Accounts  []WtWbAccount
	Total     int
	Healthy   int
	Cooling   int
	Disabled  int
	FetchedAt time.Time
}

// Account finds an account by id; nil when the panel has none.
func (status *WtWbStatus) Account(uid string) *WtWbAccount {
	if status == nil {
		return nil
	}
	for i := range status.Accounts {
		if status.Accounts[i].UID == uid {
			return &status.Accounts[i]
		}
	}
	return nil
}

// WtWbPoolInfo is what the list shows about an active WorkBuddy donation.
type WtWbPoolInfo struct {
	State        string `json:"state"`
	Message      string `json:"message"`
	Nickname     string `json:"nickname"`
	Realm        string `json:"realm"`
	Credits      int64  `json:"credits"`
	CreditsTotal int64  `json:"credits_total"`
	RetryAt      int64  `json:"retry_at"`
	Success      int64  `json:"success"`
	Failed       int64  `json:"failed"`
	CheckedIn    bool   `json:"checked_in"`
}

// WtWbPoolStateOf maps the panel's view of an account to the state the
// member sees: missing, disabled (the panel gave up on its token), paused by
// the pool's owner, cooling down after Tencent refused it, or available.
func WtWbPoolStateOf(status *WtWbStatus, uid string, now time.Time) WtWbPoolInfo {
	account := status.Account(uid)
	if account == nil {
		return WtWbPoolInfo{State: WtWbStateMissing, Message: "The pool no longer has this account"}
	}
	info := WtWbPoolInfo{
		Nickname:     account.Nickname,
		Realm:        account.Realm,
		Credits:      account.Credits,
		CreditsTotal: account.CreditsTotal,
		Success:      account.SuccessCount,
		Failed:       account.ErrTotal,
		CheckedIn:    account.CheckinDone,
	}
	switch {
	case account.Disabled:
		info.State, info.Message = WtWbStateDisabled, "The pool can no longer sign in with this account; remove it and donate again"
	case account.Paused:
		info.State, info.Message = WtWbStatePaused, "The pool owner paused this account"
	case account.Cooling:
		info.State, info.Message = WtWbStateCooldown, "Rate-limited; the pool is resting this account"
		if account.CoolKind == wtWbCoolHard {
			info.Message = "Credits used up for today"
		}
		if account.CoolRemaining > 0 {
			info.RetryAt = now.Add(time.Duration(account.CoolRemaining) * time.Second).Unix()
		}
	default:
		info.State, info.Message = WtWbStateAvailable, "Available"
	}
	return info
}

// WtWbPoolLevel is the WorkBuddy tank of the pool page: the accounts the
// panel holds and the credits left across them. FetchedAt is 0 while the
// panel has not answered.
type WtWbPoolLevel struct {
	FetchedAt    int64 `json:"fetched_at"`
	Accounts     int   `json:"accounts"`
	Healthy      int   `json:"healthy"`
	Cooling      int   `json:"cooling"`
	Credits      int64 `json:"credits"`
	CreditsTotal int64 `json:"credits_total"`
}

// WtWbPoolLevelOf sums the panel's status into the tank: credits of the
// accounts that are not disabled, against the totals the panel knows.
func WtWbPoolLevelOf(status *WtWbStatus) WtWbPoolLevel {
	level := WtWbPoolLevel{FetchedAt: status.FetchedAt.Unix(), Accounts: status.Total, Healthy: status.Healthy, Cooling: status.Cooling}
	for _, account := range status.Accounts {
		if account.Disabled {
			continue
		}
		level.Credits += max(account.Credits, 0)
		level.CreditsTotal += max(account.CreditsTotal, 0)
	}
	return level
}

// wtWbClient is the panel client: the shared transport, a timeout and no
// redirects.
func wtWbClient() *http.Client {
	client := http.Client{}
	if shared := GetHttpClient(); shared != nil {
		client = *shared
	}
	client.Timeout = wtWbTimeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	client.Jar = nil
	return &client
}

// wtWbFailure names a transport failure without its address.
func wtWbFailure(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return "timed out"
	}
	return "connection failed"
}

// wtWbDo sends one panel request with the key and returns the HTTP status
// and the capped body. ErrWtWbUnavailable covers an unconfigured pool, an
// unreachable one and a refused key. op names the call in the log.
func wtWbDo(ctx context.Context, op, method, path string, body any) (int, []byte, error) {
	if !WtWbEnabled() {
		return 0, nil, ErrWtWbUnavailable
	}
	var payload io.Reader
	if body != nil {
		encoded, err := common.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, wtWbURL()+path, payload)
	if err != nil {
		common.SysError(common.LogText("wt wb: %s request could not be built", op))
		return 0, nil, ErrWtWbUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+WtWbAPIKey())
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := wtWbClient().Do(request)
	if err != nil {
		common.SysError(common.LogText("wt wb: %s %s", op, wtWbFailure(err)))
		return 0, nil, ErrWtWbUnavailable
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, wtWbMaxBody+1))
	if err != nil || len(raw) > wtWbMaxBody {
		common.SysError(common.LogText("wt wb: %s response could not be read", op))
		return 0, nil, ErrWtWbUnavailable
	}
	if response.StatusCode == http.StatusUnauthorized {
		common.SysError(common.LogText("wt wb: %s was refused; check WT_WB_API_KEY", op))
		return 0, nil, ErrWtWbUnavailable
	}
	return response.StatusCode, raw, nil
}

// wtWbUnexpected logs an answer the site cannot use and returns the pool error.
func wtWbUnexpected(op string, status int) error {
	common.SysError(common.LogText("wt wb: %s failed with status %d", op, status))
	return ErrWtWbUnavailable
}

// wtWbFetchStatus reads the panel's status: every account with its credits
// and cooldowns, and the counts.
func wtWbFetchStatus(ctx context.Context) (*WtWbStatus, error) {
	status, raw, err := wtWbDo(ctx, "status", http.MethodGet, "/status", nil)
	if err != nil {
		return nil, err
	}
	var reply struct {
		Accounts []WtWbAccount `json:"accounts"`
		Total    int           `json:"total"`
		Healthy  int           `json:"healthy"`
		Cooling  int           `json:"cooling"`
		Disabled int           `json:"disabled"`
	}
	if status != http.StatusOK || common.Unmarshal(raw, &reply) != nil {
		return nil, wtWbUnexpected("status", status)
	}
	parsed := &WtWbStatus{Accounts: reply.Accounts, Total: reply.Total, Healthy: reply.Healthy, Cooling: reply.Cooling, Disabled: reply.Disabled, FetchedAt: time.Now()}
	if parsed.Accounts == nil {
		parsed.Accounts = []WtWbAccount{}
	}
	return parsed, nil
}

// WtWbStatusNow is the panel's status afresh, for tests of an account; the
// cache takes it too.
func WtWbStatusNow(ctx context.Context) (*WtWbStatus, error) {
	status, err := wtWbFetchStatus(ctx)
	if err != nil {
		return nil, err
	}
	wtWbStatusMu.Lock()
	wtWbStatus = status
	wtWbStatusMu.Unlock()
	return status, nil
}

// WtWbStatusCached is the status at most 15 seconds old. The lock is held
// through the request so that one call serves every member who opens the
// page at the same time.
func WtWbStatusCached(ctx context.Context) (*WtWbStatus, error) {
	wtWbStatusMu.Lock()
	defer wtWbStatusMu.Unlock()
	if wtWbStatus != nil && time.Since(wtWbStatus.FetchedAt) < wtWbStatusCacheTTL {
		return wtWbStatus, nil
	}
	status, err := wtWbFetchStatus(ctx)
	if err != nil {
		return nil, err
	}
	wtWbStatus = status
	return status, nil
}

// WtForgetWbStatus drops the cached status after the pool changed.
func WtForgetWbStatus() {
	wtWbStatusMu.Lock()
	wtWbStatus = nil
	wtWbStatusMu.Unlock()
}

// WtWbSignIn is a sign-in the panel started: Tencent's page for the member
// to open and the session the panel follows it by. The state is kept in
// the donation's record, never sent to the browser.
type WtWbSignIn struct {
	URL   string
	State string
}

// WtWbStartSignIn asks the panel to begin a sign-in of the realm. The link
// must be an https address, since the member's browser opens it.
func WtWbStartSignIn(ctx context.Context, realm string) (*WtWbSignIn, error) {
	status, raw, err := wtWbDo(ctx, "sign-in start", http.MethodPost, "/panel/api/login/start", map[string]string{"realm": realm})
	if err != nil {
		return nil, err
	}
	if status == http.StatusBadGateway {
		return nil, ErrWtWbSignInLink
	}
	var reply struct {
		OK    bool   `json:"ok"`
		URL   string `json:"url"`
		State string `json:"state"`
	}
	if status != http.StatusOK || common.Unmarshal(raw, &reply) != nil || !reply.OK || reply.State == "" || len(reply.State) > wtWbStateMax {
		return nil, wtWbUnexpected("sign-in start", status)
	}
	link, parseErr := url.Parse(reply.URL)
	if parseErr != nil || link.Scheme != "https" || link.Host == "" {
		return nil, wtWbUnexpected("sign-in start", status)
	}
	return &WtWbSignIn{URL: reply.URL, State: reply.State}, nil
}

// WtWbSignInResult is one poll of a sign-in: nothing yet, or the account
// the panel added to the pool.
type WtWbSignInResult struct {
	Done         bool
	UID          string
	Nickname     string
	Realm        string
	Credits      int64
	CreditsTotal int64
}

// WtWbPollSignIn asks the panel whether the sign-in finished. Once it has,
// the panel holds the account: the result names it. A sign-in the panel
// forgot is ErrWtWbSignInExpired; Tencent not answering is
// ErrWtWbUnavailable, and the caller asks again.
func WtWbPollSignIn(ctx context.Context, state string) (*WtWbSignInResult, error) {
	status, raw, err := wtWbDo(ctx, "sign-in poll", http.MethodGet, "/panel/api/login/poll?state="+url.QueryEscape(state), nil)
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusNotFound:
		return nil, ErrWtWbSignInExpired
	case http.StatusBadGateway:
		return nil, ErrWtWbUnavailable
	case http.StatusOK:
	default:
		return nil, wtWbUnexpected("sign-in poll", status)
	}
	var reply struct {
		Done         bool   `json:"done"`
		UID          string `json:"uid"`
		Nickname     string `json:"nickname"`
		Realm        string `json:"realm"`
		Credits      int64  `json:"credits"`
		CreditsTotal int64  `json:"credits_total"`
	}
	if common.Unmarshal(raw, &reply) != nil {
		return nil, wtWbUnexpected("sign-in poll", status)
	}
	if !reply.Done {
		return &WtWbSignInResult{}, nil
	}
	if !wtWbValidUID(reply.UID) {
		return nil, wtWbUnexpected("sign-in poll", status)
	}
	return &WtWbSignInResult{Done: true, UID: reply.UID, Nickname: reply.Nickname, Realm: reply.Realm, Credits: reply.Credits, CreditsTotal: reply.CreditsTotal}, nil
}

// WtWbRemoveAccount takes an account out of the panel, which deletes its
// token file; an account the panel no longer has counts as removed.
func WtWbRemoveAccount(ctx context.Context, uid string) error {
	if !wtWbValidUID(uid) {
		return nil
	}
	status, _, err := wtWbDo(ctx, "remove", http.MethodPost, "/panel/api/accounts/"+url.PathEscape(uid)+"/remove", nil)
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusNotFound {
		return nil
	}
	return wtWbUnexpected("remove", status)
}

// WtWbListModels is what the panel serves right now, as /v1/models names
// it: empty while the pool holds no account.
func WtWbListModels(ctx context.Context) ([]string, error) {
	status, raw, err := wtWbDo(ctx, "models", http.MethodGet, "/v1/models", nil)
	if err != nil {
		return nil, err
	}
	var reply struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status != http.StatusOK || common.Unmarshal(raw, &reply) != nil {
		return nil, wtWbUnexpected("models", status)
	}
	names := make([]string, 0, len(reply.Data))
	for _, entry := range reply.Data {
		if name := strings.TrimSpace(entry.ID); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}
