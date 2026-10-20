package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// wind-tide: client of the aistudio-api management API that holds the
// donated AI Studio sign-ins (wind-tide/API.md §15). The console password and
// the relay key never leave this process, every path is fixed, redirects are
// not followed and responses are capped. Cookie text is parsed, sent once to
// the pool and forgotten: it is never logged, stored or echoed. Log lines
// carry the operation and the HTTP status only.

// The relay models of the group and their upstream names are the
// WtAisProvider rows of model.WtAisModels; every one carries WtAisModelPrefix.
const (
	WtAisModelPrefix    = "AIS PG-"
	WtAisGroup          = "AIS PG"
	WtAisGroupLabel     = "AIS playground pool"
	WtAisChannelName    = "AIS playground"
	WtAisChannelTag     = "wt-ais"
	WtAisCookieMaxBytes = 64 * 1024

	wtAisTimeout          = 15 * time.Second
	wtAisMaxBody          = 1 << 20
	wtAisRotationCacheTTL = 15 * time.Second
	wtAisFingerprintHex   = 32
)

// The pool states of an active AI Studio donation (wind-tide/API.md §15).
const (
	WtAisStateMissing   = "missing"
	WtAisStateExpired   = "expired"
	WtAisStateCooldown  = "cooldown"
	WtAisStateExhausted = "exhausted"
	WtAisStateAvailable = "available"
)

// The outcomes of a detect-email call that are not transport failures.
const (
	WtAisDetectOK       = "ok"        // the pool read the account's email
	WtAisDetectEmpty    = "empty"     // the pool could not read the page yet
	WtAisDetectNotReady = "not_ready" // the controlled browser is not ready
	WtAisDetectGone     = "gone"      // the pool no longer has the account
)

var (
	// ErrWtAisUnavailable stands for every failure to talk to aistudio-api.
	ErrWtAisUnavailable = common.NewMessage("The pool is not answering; try again later")
	// ErrWtAisNotConfigured is the answer while the AIS variables are not set.
	ErrWtAisNotConfigured = common.NewMessage("The AIS playground pool is not configured")
)

// wtAisFingerprintCookies are the sign-in cookies, in order of preference,
// whose value identifies a Google account.
var wtAisFingerprintCookies = []string{"__Secure-1PSID", "SID", "SAPISID", "__Secure-1PAPISID"}

// wtPacific is the upstream quota day of AI Studio; without zone data the
// summer offset keeps the day roughly right.
var wtPacific = func() *time.Location {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.FixedZone("UTC-7", -7*60*60)
	}
	return location
}()

var (
	wtAisRotationMu       sync.Mutex
	wtAisRotationCache    map[string]*WtAisRotationEntry
	wtAisRotationCachedAt time.Time
)

// WtAisEnabled reports whether the site is wired to an AI Studio pool: the
// address, the console password and the relay key are all set.
func WtAisEnabled() bool {
	return wtAisURL() != "" && wtAisPassword() != "" && WtAisAPIKey() != ""
}

func wtAisURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("WT_AIS_URL")), "/")
}

func wtAisPassword() string {
	return strings.TrimSpace(os.Getenv("WT_AIS_PASSWORD"))
}

// WtAisAPIKey is the relay key registered in aistudio-api; it becomes the
// key of the site's AIS channel.
func WtAisAPIKey() string {
	return strings.TrimSpace(os.Getenv("WT_AIS_API_KEY"))
}

// WtAisRotationEntry is what the pool reports about one account's rotation,
// the fields the site reads. Times are Unix seconds as floats; 0 means none.
type WtAisRotationEntry struct {
	AccountID          string             `json:"account_id"`
	Requests           int64              `json:"requests"`
	Success            int64              `json:"success"`
	RateLimited        int64              `json:"rate_limited"`
	Errors             int64              `json:"errors"`
	AuthErrors         int64              `json:"auth_errors"`
	AuthCooldown       float64            `json:"auth_cooldown"`
	SessionExpired     bool               `json:"session_expired"`
	ModelCooldowns     map[string]float64 `json:"model_cooldowns"`
	ModelRequests      map[string]int64   `json:"model_requests"`
	ModelRequestsDates map[string]string  `json:"model_requests_dates"`
}

// WtAisCooling is one upstream model whose quota the account used up, with
// the time the pool will try it again.
type WtAisCooling struct {
	Model   string `json:"model"`
	RetryAt int64  `json:"retry_at"`
}

// WtAisPoolInfo is what the list shows about an active AI Studio donation.
// RequestsToday sums today's requests of every model of the group; Cooling
// lists the models whose quota is used up, in table order.
type WtAisPoolInfo struct {
	State         string         `json:"state"`
	Message       string         `json:"message"`
	RetryAt       int64          `json:"retry_at"`
	Success       int64          `json:"success"`
	Failed        int64          `json:"failed"`
	RateLimited   int64          `json:"rate_limited"`
	RequestsToday int64          `json:"requests_today"`
	Cooling       []WtAisCooling `json:"cooling"`
}

// WtAisPoolStateOf maps an account's rotation entry to the state the member
// sees; a nil entry means the pool no longer has the account. The first
// matching condition wins: expired session, sign-in cooldown, upstream quota
// cooldown of every model of the group (retry_at is the earliest), else
// available.
func WtAisPoolStateOf(entry *WtAisRotationEntry, now time.Time) WtAisPoolInfo {
	info := WtAisPoolInfo{Cooling: []WtAisCooling{}}
	if entry == nil {
		info.State, info.Message = WtAisStateMissing, "The pool no longer has this account"
		return info
	}
	info.Success, info.Failed, info.RateLimited = entry.Success, entry.Errors+entry.AuthErrors, entry.RateLimited
	today := now.In(wtPacific).Format("2006-01-02")
	unix := float64(now.Unix())
	specs := model.WtAisModelsOf(model.WtAisProvider)
	for _, spec := range specs {
		if entry.ModelRequestsDates[spec.Upstream] == today {
			info.RequestsToday += entry.ModelRequests[spec.Upstream]
		}
		if cooldown := entry.ModelCooldowns[spec.Upstream]; cooldown > unix {
			info.Cooling = append(info.Cooling, WtAisCooling{Model: spec.Upstream, RetryAt: int64(cooldown)})
		}
	}
	switch {
	case entry.SessionExpired:
		info.State, info.Message = WtAisStateExpired, "The pool can no longer sign in with this account; remove it and donate again"
	case entry.AuthCooldown > unix:
		info.State, info.Message, info.RetryAt = WtAisStateCooldown, "The pool paused this account after a sign-in error", int64(entry.AuthCooldown)
	case len(info.Cooling) > 0 && len(info.Cooling) == len(specs):
		earliest := info.Cooling[0].RetryAt
		for _, cooling := range info.Cooling[1:] {
			earliest = min(earliest, cooling.RetryAt)
		}
		info.State, info.Message, info.RetryAt = WtAisStateExhausted, "Today's upstream quota of this account is used up", earliest
	default:
		info.State, info.Message = WtAisStateAvailable, "Available"
	}
	return info
}

// WtAisParseCookies turns pasted cookie text into the header string the pool
// imports and the fingerprint of the Google account it signs in. Accepted
// forms: a header string (k=v; k2=v2), a JSON array of {name, value}, or the
// Android extractor bundle, whose first account's cookies are used. ok is
// false when no sign-in cookie is present. The text is never kept.
func WtAisParseCookies(raw string) (header, fingerprint string, ok bool) {
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "{") {
		var bundle struct {
			Accounts []struct {
				Cookies json.RawMessage `json:"cookies"`
			} `json:"accounts"`
			Profiles []struct {
				Cookies json.RawMessage `json:"cookies"`
			} `json:"profiles"`
		}
		if common.UnmarshalJsonStr(text, &bundle) != nil {
			return "", "", false
		}
		items := bundle.Accounts
		if len(items) == 0 {
			items = bundle.Profiles
		}
		if len(items) == 0 {
			return "", "", false
		}
		var asString string
		if common.Unmarshal(items[0].Cookies, &asString) == nil {
			text = strings.TrimSpace(asString)
		} else {
			text = strings.TrimSpace(string(items[0].Cookies))
		}
	}
	pairs := wtAisCookiePairs(text)
	if len(pairs) == 0 {
		return "", "", false
	}
	for _, name := range wtAisFingerprintCookies {
		for _, pair := range pairs {
			if pair[0] != name || pair[1] == "" {
				continue
			}
			digest := sha256.Sum256([]byte(pair[1]))
			fingerprint = hex.EncodeToString(digest[:])[:wtAisFingerprintHex]
			break
		}
		if fingerprint != "" {
			break
		}
	}
	if fingerprint == "" {
		return "", "", false
	}
	parts := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		parts = append(parts, pair[0]+"="+pair[1])
	}
	return strings.Join(parts, "; "), fingerprint, true
}

// wtAisCookiePairs reads name/value pairs from a JSON array of cookie objects
// or from a header string.
func wtAisCookiePairs(text string) [][2]string {
	pairs := [][2]string{}
	if strings.HasPrefix(text, "[") {
		var cookies []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if common.UnmarshalJsonStr(text, &cookies) != nil {
			return nil
		}
		for _, cookie := range cookies {
			name := strings.TrimSpace(cookie.Name)
			if name != "" {
				pairs = append(pairs, [2]string{name, strings.TrimSpace(cookie.Value)})
			}
		}
		return pairs
	}
	for part := range strings.SplitSeq(text, ";") {
		name, value, found := strings.Cut(strings.TrimSpace(part), "=")
		name = strings.TrimSpace(name)
		if !found || name == "" {
			continue
		}
		pairs = append(pairs, [2]string{name, strings.TrimSpace(value)})
	}
	return pairs
}

// wtAisDo sends one management request and returns the HTTP status and the
// (capped) body of the answer. ErrWtAisUnavailable covers an unconfigured
// pool, an unreachable one, a redirect, an oversized body and a wrong
// password. op names the call in the log.
func wtAisDo(ctx context.Context, op, method, path string, body any) (int, []byte, error) {
	if !WtAisEnabled() {
		return 0, nil, ErrWtAisUnavailable
	}
	var payload io.Reader
	if body != nil {
		encoded, err := common.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, wtAisURL()+path, payload)
	if err != nil {
		common.SysError(common.LogText("wt ais: %s request could not be built", op))
		return 0, nil, ErrWtAisUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+wtAisPassword())
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := http.Client{}
	if shared := GetHttpClient(); shared != nil {
		client = *shared
	}
	client.Timeout = wtAisTimeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	response, err := client.Do(request)
	if err != nil {
		reason := "connection failed"
		var urlErr *url.Error
		if errors.As(err, &urlErr) && urlErr.Timeout() {
			reason = "timed out"
		}
		common.SysError(common.LogText("wt ais: %s %s", op, reason))
		return 0, nil, ErrWtAisUnavailable
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, wtAisMaxBody+1))
	if err != nil || len(raw) > wtAisMaxBody {
		common.SysError(common.LogText("wt ais: %s response could not be read", op))
		return 0, nil, ErrWtAisUnavailable
	}
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		common.SysError(common.LogText("wt ais: %s failed with status %d; check WT_AIS_PASSWORD", op, response.StatusCode))
		return response.StatusCode, raw, ErrWtAisUnavailable
	case response.StatusCode >= 300 && response.StatusCode < 400:
		common.SysError(common.LogText("wt ais: %s failed with status %d", op, response.StatusCode))
		return response.StatusCode, raw, ErrWtAisUnavailable
	}
	return response.StatusCode, raw, nil
}

// wtAisUnexpected logs an answer the site cannot use and returns the pool error.
func wtAisUnexpected(op string, status int) error {
	common.SysError(common.LogText("wt ais: %s failed with status %d", op, status))
	return ErrWtAisUnavailable
}

// WtAisImportCookies hands a member's cookie header to the pool, which signs
// the account in and returns its account id. The pool answering 400 means
// nothing in the text was a cookie.
func WtAisImportCookies(ctx context.Context, cookies string, userID int) (string, error) {
	suffix := make([]byte, 2)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	body := map[string]string{
		"cookies":   cookies,
		"name":      "wt-" + strconv.Itoa(userID) + "-" + hex.EncodeToString(suffix),
		"auth_user": "0",
	}
	status, raw, err := wtAisDo(ctx, "import", http.MethodPost, "/accounts/import-cookies", body)
	if err != nil {
		return "", err
	}
	if status == http.StatusBadRequest {
		return "", model.ErrWtAisCookie
	}
	var reply struct {
		AccountID string `json:"account_id"`
	}
	if status != http.StatusOK || common.Unmarshal(raw, &reply) != nil || strings.TrimSpace(reply.AccountID) == "" {
		return "", wtAisUnexpected("import", status)
	}
	return strings.TrimSpace(reply.AccountID), nil
}

// WtAisDetectEmail makes the pool switch its browser to the account and read
// the signed-in email: the liveness check of an AI Studio account. The
// outcome tells an unreadable page, a browser that is not ready and a lost
// account apart; err is a transport failure.
func WtAisDetectEmail(ctx context.Context, accountID string) (outcome, email string, err error) {
	status, raw, err := wtAisDo(ctx, "detect-email", http.MethodPost, "/accounts/"+url.PathEscape(accountID)+"/detect-email", nil)
	if err != nil {
		return "", "", err
	}
	switch status {
	case http.StatusOK:
		var reply struct {
			Email *string `json:"email"`
		}
		if common.Unmarshal(raw, &reply) != nil {
			return "", "", wtAisUnexpected("detect-email", status)
		}
		if reply.Email == nil || strings.TrimSpace(*reply.Email) == "" {
			return WtAisDetectEmpty, "", nil
		}
		return WtAisDetectOK, strings.TrimSpace(*reply.Email), nil
	case http.StatusNotFound:
		return WtAisDetectGone, "", nil
	case http.StatusServiceUnavailable:
		return WtAisDetectNotReady, "", nil
	}
	return "", "", wtAisUnexpected("detect-email", status)
}

// WtAisDeleteAccount removes an account from the pool; one the pool no
// longer has counts as removed.
func WtAisDeleteAccount(ctx context.Context, accountID string) error {
	status, _, err := wtAisDo(ctx, "delete", http.MethodDelete, "/accounts/"+url.PathEscape(accountID), nil)
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusNotFound {
		return nil
	}
	return wtAisUnexpected("delete", status)
}

// WtAisRotation is the pool's rotation table by account id, at most 15
// seconds old. The lock is held through the request so that one call serves
// every member who opens the page at the same time.
func WtAisRotation(ctx context.Context) (map[string]*WtAisRotationEntry, error) {
	wtAisRotationMu.Lock()
	defer wtAisRotationMu.Unlock()
	if wtAisRotationCache != nil && time.Since(wtAisRotationCachedAt) < wtAisRotationCacheTTL {
		return wtAisRotationCache, nil
	}
	status, raw, err := wtAisDo(ctx, "rotation", http.MethodGet, "/rotation/accounts", nil)
	if err != nil {
		return nil, err
	}
	var entries map[string]*WtAisRotationEntry
	if status != http.StatusOK || common.Unmarshal(raw, &entries) != nil {
		return nil, wtAisUnexpected("rotation", status)
	}
	if entries == nil {
		entries = map[string]*WtAisRotationEntry{}
	}
	wtAisRotationCache, wtAisRotationCachedAt = entries, time.Now()
	return entries, nil
}

// WtForgetAisRotation drops the rotation table after the pool changed.
func WtForgetAisRotation() {
	wtAisRotationMu.Lock()
	wtAisRotationCache = nil
	wtAisRotationMu.Unlock()
}
