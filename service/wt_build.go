package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// wind-tide: client of the AIStudioToAPI console that holds the donated Build
// sign-ins (wind-tide/API.md §15). The console is session-based: the site
// signs in with the console password, keeps the session cookie in memory and
// signs in again when the console forgot it (a restart). The password and the
// relay key never leave this process, every path is fixed, redirects are not
// followed and responses are capped. Cookie text becomes the pool's auth
// file, travels once and is forgotten: it is never logged, stored or echoed.
// Log lines carry the operation and the HTTP status only.

// The relay model of the BUILD group and its upstream name are the
// WtBuildProvider rows of model.WtAisModels; every one carries
// WtBuildModelPrefix.
const (
	WtBuildModelPrefix = "Build-"
	WtBuildGroup       = "BUILD"
	WtBuildGroupLabel  = "BUILD pool"
	WtBuildChannelName = "BUILD"
	WtBuildChannelTag  = "wt-build"

	wtBuildTimeout        = 20 * time.Second
	wtBuildMaxBody        = 1 << 20
	wtBuildStatusCacheTTL = 15 * time.Second
	// wtBuildCookieLife is the expiry written into the auth file; Google's
	// own expiry governs the real sign-in.
	wtBuildCookieLife = 180 * 24 * time.Hour
)

// The pool states of an active Build donation (wind-tide/API.md §15).
const (
	WtBuildStateMissing   = "missing"
	WtBuildStateExpired   = "expired"
	WtBuildStateInvalid   = "invalid"
	WtBuildStateAvailable = "available"
)

var (
	// ErrWtBuildUnavailable stands for every failure to talk to the console.
	ErrWtBuildUnavailable = common.NewMessage("The pool is not answering; try again later")
	// ErrWtBuildBusy is the console switching accounts; the call can be retried.
	ErrWtBuildBusy = common.NewMessage("The pool is busy switching accounts; try again in a moment")
	// ErrWtBuildNotConfigured is the answer while the BUILD variables are not set.
	ErrWtBuildNotConfigured = common.NewMessage("The BUILD pool is not configured")
)

// wtBuildScriptCookies are the cookies the AI Studio page reads from script
// (for its SAPISIDHASH authorization); every other cookie is HttpOnly.
var wtBuildScriptCookies = map[string]bool{"SAPISID": true, "APISID": true, "__Secure-1PAPISID": true, "__Secure-3PAPISID": true}

var (
	wtBuildSessionMu sync.Mutex
	wtBuildSession   http.CookieJar
	wtBuildSessionAt string // the console address the session belongs to

	wtBuildStatusMu sync.Mutex
	wtBuildStatus   *WtBuildStatus
)

// WtBuildEnabled reports whether the site is wired to a BUILD pool: the
// address, the console password and the relay key are all set.
func WtBuildEnabled() bool {
	return wtBuildURL() != "" && wtBuildPassword() != "" && WtBuildAPIKey() != ""
}

func wtBuildURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("WT_BUILD_URL")), "/")
}

func wtBuildPassword() string {
	return strings.TrimSpace(os.Getenv("WT_BUILD_PASSWORD"))
}

// WtBuildAPIKey is one of the relay keys of AIStudioToAPI; it becomes the key
// of the site's BUILD channel.
func WtBuildAPIKey() string {
	return strings.TrimSpace(os.Getenv("WT_BUILD_API_KEY"))
}

// WtBuildAccount is what the console reports about one auth file.
type WtBuildAccount struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	HasContext bool   `json:"hasContext"`
	IsExpired  bool   `json:"isExpired"`
	IsInvalid  bool   `json:"isInvalid"`
	IsRotation bool   `json:"isRotation"`
}

// WtBuildStatus is the part of the console's status the site reads.
// CurrentIndex is -1 while the console serves nobody.
type WtBuildStatus struct {
	Accounts     []WtBuildAccount
	CurrentIndex int
	Busy         bool
	FetchedAt    time.Time
}

// Account finds an auth file by index; nil when the console has none.
func (status *WtBuildStatus) Account(index int) *WtBuildAccount {
	if status == nil {
		return nil
	}
	for i := range status.Accounts {
		if status.Accounts[i].Index == index {
			return &status.Accounts[i]
		}
	}
	return nil
}

// WtBuildPoolInfo is what the list shows about an active Build donation.
type WtBuildPoolInfo struct {
	State      string `json:"state"`
	Message    string `json:"message"`
	Index      int    `json:"index"`
	Current    bool   `json:"current"`
	HasContext bool   `json:"has_context"`
}

// WtBuildPoolStateOf maps the console's view of an auth file to the state the
// member sees: missing, invalid (the file could not be read), expired (the
// sign-in bounced to Google's login) or available.
func WtBuildPoolStateOf(status *WtBuildStatus, index int) WtBuildPoolInfo {
	info := WtBuildPoolInfo{Index: index}
	account := status.Account(index)
	switch {
	case account == nil:
		info.State, info.Message = WtBuildStateMissing, "The pool no longer has this account"
	case account.IsInvalid:
		info.State, info.Message = WtBuildStateInvalid, "The pool could not read this account; remove it and donate again"
	case account.IsExpired:
		info.State, info.Message = WtBuildStateExpired, "The pool can no longer sign in with this account; remove it and donate again"
	default:
		info.State, info.Message = WtBuildStateAvailable, "Available"
		info.Current, info.HasContext = status.CurrentIndex == index, account.HasContext
	}
	return info
}

// WtBuildStorageState builds the auth file the console loads: the browser
// storage state of the sign-in cookies, every one on .google.com, named after
// the member so the owner can tell donations apart in the console. The name
// is never an email, so the console never folds two members' files into one.
// Host-only cookies (__Host-) cannot travel this way and are left out; the
// sign-in does not need them.
func WtBuildStorageState(cookieHeader string, userID int) (map[string]any, error) {
	pairs := wtAisCookiePairs(cookieHeader)
	cookies := make([]map[string]any, 0, len(pairs))
	expires := float64(time.Now().Add(wtBuildCookieLife).Unix())
	for _, pair := range pairs {
		if pair[1] == "" || strings.HasPrefix(pair[0], "__Host-") {
			continue
		}
		cookies = append(cookies, map[string]any{
			"name":     pair[0],
			"value":    pair[1],
			"domain":   ".google.com",
			"path":     "/",
			"expires":  expires,
			"httpOnly": !wtBuildScriptCookies[pair[0]],
			"secure":   true,
			"sameSite": "None",
		})
	}
	if len(cookies) == 0 {
		return nil, model.ErrWtAisCookie
	}
	suffix := make([]byte, 2)
	if _, err := rand.Read(suffix); err != nil {
		return nil, err
	}
	return map[string]any{
		"cookies":     cookies,
		"origins":     []any{},
		"accountName": "wt-" + strconv.Itoa(userID) + "-" + hex.EncodeToString(suffix),
	}, nil
}

// wtBuildClient is the console client: the shared transport, the session
// jar, a timeout and no redirects, since the console answers sign-ins with
// redirects the site reads as status codes.
func wtBuildClient() *http.Client {
	client := http.Client{}
	if shared := GetHttpClient(); shared != nil {
		client = *shared
	}
	client.Timeout = wtBuildTimeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	client.Jar = wtBuildSessionJar()
	return &client
}

// wtBuildSessionJar is the jar of the console session, new when the address
// changed or the session was dropped.
func wtBuildSessionJar() http.CookieJar {
	wtBuildSessionMu.Lock()
	defer wtBuildSessionMu.Unlock()
	if wtBuildSession == nil || wtBuildSessionAt != wtBuildURL() {
		wtBuildSession, _ = cookiejar.New(nil)
		wtBuildSessionAt = wtBuildURL()
	}
	return wtBuildSession
}

// wtBuildSignedIn reports whether the jar holds a cookie for the console.
func wtBuildSignedIn() bool {
	target, err := url.Parse(wtBuildURL())
	if err != nil {
		return false
	}
	return len(wtBuildSessionJar().Cookies(target)) > 0
}

func wtBuildForgetSession() {
	wtBuildSessionMu.Lock()
	wtBuildSession = nil
	wtBuildSessionMu.Unlock()
}

// wtBuildSignIn posts the console password; the console answers a redirect
// to / with the session cookie, or to /login?error=… when it refuses.
func wtBuildSignIn(ctx context.Context) error {
	encoded, err := common.Marshal(map[string]string{"password": wtBuildPassword()})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, wtBuildURL()+"/login", bytes.NewReader(encoded))
	if err != nil {
		common.SysError(common.LogText("wt build: sign-in request could not be built"))
		return ErrWtBuildUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := wtBuildClient().Do(request)
	if err != nil {
		common.SysError(common.LogText("wt build: sign-in %s", wtBuildFailure(err)))
		return ErrWtBuildUnavailable
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, wtBuildMaxBody))
	redirected := response.StatusCode >= 300 && response.StatusCode < 400
	if redirected && !strings.Contains(response.Header.Get("Location"), "error") && wtBuildSignedIn() {
		return nil
	}
	common.SysError(common.LogText("wt build: sign-in failed with status %d; check WT_BUILD_PASSWORD", response.StatusCode))
	return ErrWtBuildUnavailable
}

// wtBuildFailure names a transport failure without its address.
func wtBuildFailure(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return "timed out"
	}
	return "connection failed"
}

// wtBuildDo sends one console request inside the session, signing in first
// when there is no session and once more when the console forgot it. It
// returns the HTTP status and the capped body; ErrWtBuildUnavailable covers
// an unconfigured pool, an unreachable one and a refused sign-in. op names
// the call in the log.
func wtBuildDo(ctx context.Context, op, method, path string, body any) (int, []byte, error) {
	if !WtBuildEnabled() {
		return 0, nil, ErrWtBuildUnavailable
	}
	var encoded []byte
	if body != nil {
		var err error
		if encoded, err = common.Marshal(body); err != nil {
			return 0, nil, err
		}
	}
	if !wtBuildSignedIn() {
		if err := wtBuildSignIn(ctx); err != nil {
			return 0, nil, err
		}
	}
	for attempt := range 2 {
		status, raw, err := wtBuildSend(ctx, op, method, path, encoded)
		if err != nil {
			return 0, nil, err
		}
		if status != http.StatusUnauthorized && (status < 300 || status >= 400) {
			return status, raw, nil
		}
		if attempt == 1 {
			break
		}
		// The console restarted and forgot the session: sign in once more.
		wtBuildForgetSession()
		if err := wtBuildSignIn(ctx); err != nil {
			return 0, nil, err
		}
	}
	common.SysError(common.LogText("wt build: %s was refused after a fresh sign-in", op))
	return 0, nil, ErrWtBuildUnavailable
}

func wtBuildSend(ctx context.Context, op, method, path string, encoded []byte) (int, []byte, error) {
	var payload io.Reader
	if encoded != nil {
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, wtBuildURL()+path, payload)
	if err != nil {
		common.SysError(common.LogText("wt build: %s request could not be built", op))
		return 0, nil, ErrWtBuildUnavailable
	}
	request.Header.Set("Accept", "application/json")
	if encoded != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := wtBuildClient().Do(request)
	if err != nil {
		common.SysError(common.LogText("wt build: %s %s", op, wtBuildFailure(err)))
		return 0, nil, ErrWtBuildUnavailable
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, wtBuildMaxBody+1))
	if err != nil || len(raw) > wtBuildMaxBody {
		common.SysError(common.LogText("wt build: %s response could not be read", op))
		return 0, nil, ErrWtBuildUnavailable
	}
	return response.StatusCode, raw, nil
}

// wtBuildUnexpected logs an answer the site cannot use and returns the pool error.
func wtBuildUnexpected(op string, status int) error {
	common.SysError(common.LogText("wt build: %s failed with status %d", op, status))
	return ErrWtBuildUnavailable
}

// wtBuildFetchStatus reads the console's status: its auth files, the account
// it serves and whether it is switching. The console reloads its auth files
// on every status call, so the answer is current.
func wtBuildFetchStatus(ctx context.Context) (*WtBuildStatus, error) {
	status, raw, err := wtBuildDo(ctx, "status", http.MethodGet, "/api/status", nil)
	if err != nil {
		return nil, err
	}
	var reply struct {
		Status struct {
			AccountDetails   []WtBuildAccount `json:"accountDetails"`
			CurrentAuthIndex *int             `json:"currentAuthIndex"`
			IsSystemBusy     bool             `json:"isSystemBusy"`
		} `json:"status"`
	}
	if status != http.StatusOK || common.Unmarshal(raw, &reply) != nil {
		return nil, wtBuildUnexpected("status", status)
	}
	parsed := &WtBuildStatus{Accounts: reply.Status.AccountDetails, CurrentIndex: -1, Busy: reply.Status.IsSystemBusy, FetchedAt: time.Now()}
	if parsed.Accounts == nil {
		parsed.Accounts = []WtBuildAccount{}
	}
	if reply.Status.CurrentAuthIndex != nil {
		parsed.CurrentIndex = *reply.Status.CurrentAuthIndex
	}
	return parsed, nil
}

// WtBuildStatusNow is the console's status afresh, for verifications and
// tests; the cache takes it too.
func WtBuildStatusNow(ctx context.Context) (*WtBuildStatus, error) {
	status, err := wtBuildFetchStatus(ctx)
	if err != nil {
		return nil, err
	}
	wtBuildStatusMu.Lock()
	wtBuildStatus = status
	wtBuildStatusMu.Unlock()
	return status, nil
}

// WtBuildStatusCached is the status at most 15 seconds old. The lock is held
// through the request so that one call serves every member who opens the
// page at the same time.
func WtBuildStatusCached(ctx context.Context) (*WtBuildStatus, error) {
	wtBuildStatusMu.Lock()
	defer wtBuildStatusMu.Unlock()
	if wtBuildStatus != nil && time.Since(wtBuildStatus.FetchedAt) < wtBuildStatusCacheTTL {
		return wtBuildStatus, nil
	}
	status, err := wtBuildFetchStatus(ctx)
	if err != nil {
		return nil, err
	}
	wtBuildStatus = status
	return status, nil
}

// WtForgetBuildStatus drops the cached status after the pool changed.
func WtForgetBuildStatus() {
	wtBuildStatusMu.Lock()
	wtBuildStatus = nil
	wtBuildStatusMu.Unlock()
}

// WtBuildUploadAccount hands an auth file to the console, which saves it
// under the next free index and warms a browser context for it in the
// background. It returns that index; a console busy switching accounts
// answers ErrWtBuildBusy.
func WtBuildUploadAccount(ctx context.Context, state map[string]any) (int, error) {
	status, raw, err := wtBuildDo(ctx, "upload", http.MethodPost, "/api/files", map[string]any{"content": state})
	if err != nil {
		return 0, err
	}
	if status == http.StatusConflict {
		return 0, ErrWtBuildBusy
	}
	var reply struct {
		Filename string `json:"filename"`
	}
	index, ok := 0, false
	if status == http.StatusOK && common.Unmarshal(raw, &reply) == nil {
		index, ok = wtBuildIndexOf(reply.Filename)
	}
	if !ok {
		return 0, wtBuildUnexpected("upload", status)
	}
	return index, nil
}

// wtBuildIndexOf reads the index out of an auth file name, "auth-7.json".
func wtBuildIndexOf(filename string) (int, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(filename), "auth-")
	if !ok {
		return 0, false
	}
	digits, ok := strings.CutSuffix(rest, ".json")
	if !ok {
		return 0, false
	}
	index, err := strconv.Atoi(digits)
	return index, err == nil && index >= 0
}

// WtBuildSwitchTo asks the console to serve the account now, which makes it
// sign the account in; a console that is busy or refuses is reported, not
// retried.
func WtBuildSwitchTo(ctx context.Context, index int) error {
	status, _, err := wtBuildDo(ctx, "switch", http.MethodPut, "/api/accounts/current", map[string]any{"targetIndex": index})
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusConflict:
		return ErrWtBuildBusy
	}
	return wtBuildUnexpected("switch", status)
}

// WtBuildDeleteAccount removes an auth file from the console, even the one
// it is serving; a file the console no longer has counts as removed.
func WtBuildDeleteAccount(ctx context.Context, index int) error {
	status, _, err := wtBuildDo(ctx, "delete", http.MethodDelete, "/api/accounts/"+strconv.Itoa(index)+"?force=true", nil)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK, http.StatusNotFound:
		return nil
	case http.StatusConflict:
		return ErrWtBuildBusy
	}
	return wtBuildUnexpected("delete", status)
}
