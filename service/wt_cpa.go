package service

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// wind-tide: client of the CLIProxyAPI (CPA) management API that holds the
// donated Antigravity accounts (wind-tide/API.md §14; design in
// wind-tide/specs/2026-10-17-antigravity-donation-design.md §6). The
// management key never leaves this process, every path is fixed, redirects
// are not followed and responses are capped. Log lines carry the operation
// and the HTTP status only: never an address, a state, a code or an email.

const (
	// WtDonationProvider is the only provider members can donate.
	WtDonationProvider = "antigravity"

	wtCPAPrefix       = "/v8/management"
	wtCPATimeout      = 15 * time.Second
	wtCPAMaxBody      = 1 << 20
	wtCPAListPageSize = 100
	wtCPAListMaxPages = 10

	// wtCPAUnknownStateError is CPA's answer, with HTTP 200, for a state it
	// has forgotten: it keeps finished sessions for 60 seconds only.
	wtCPAUnknownStateError = "unknown or expired state"

	wtGoogleQuotaHost     = "https://daily-cloudcode-pa.googleapis.com"
	wtGoogleQuotaFallback = "https://cloudcode-pa.googleapis.com"
	wtGoogleQuotaPath     = "/v1internal:retrieveUserQuotaSummary"
	wtGoogleTierPath      = "/v1internal:loadCodeAssist"
	wtGoogleUserAgent     = "antigravity/hub/2.23.0 darwin/arm64"
)

// The states of a CPA authorization session (GET /oauth/status). Unknown is
// the site's name for a state CPA no longer remembers.
const (
	WtCPAOAuthWait    = "wait"
	WtCPAOAuthOK      = "ok"
	WtCPAOAuthError   = "error"
	WtCPAOAuthUnknown = "unknown"
)

var (
	// ErrWtPoolUnavailable stands for every failure to talk to CPA.
	ErrWtPoolUnavailable = common.NewMessage("The account pool is temporarily unavailable, try again later")
	// ErrWtUsageUnavailable is a usage query Google did not answer.
	ErrWtUsageUnavailable = common.NewMessage("Could not fetch usage for this account")
)

// WtDonationEnabled reports whether the site is wired to an account pool:
// both WT_CPA_URL and WT_CPA_MANAGEMENT_KEY are set.
func WtDonationEnabled() bool {
	return strings.TrimSpace(os.Getenv("WT_CPA_URL")) != "" && strings.TrimSpace(os.Getenv("WT_CPA_MANAGEMENT_KEY")) != ""
}

// WtCredentialName is the file name CPA gives the credential of an
// Antigravity account.
func WtCredentialName(email string) string {
	return "antigravity-" + strings.TrimSpace(email) + ".json"
}

// WtCPACooldown is one running cooldown timer of a credential.
type WtCPACooldown struct {
	RetryAt          string `json:"retry_at"`
	RemainingSeconds int64  `json:"remaining_seconds"`
}

// WtCPACredential is one entry of CPA's credential list, the fields the site
// reads. Times are RFC 3339 strings; Success and Failed count the requests
// CPA made with the credential since CPA last started.
type WtCPACredential struct {
	Name           string          `json:"name"`
	Email          string          `json:"email"`
	AuthIndex      string          `json:"auth_index"`
	ProjectID      string          `json:"project_id"`
	Provider       string          `json:"provider"`
	Status         string          `json:"status"`
	StatusMessage  string          `json:"status_message"`
	Disabled       bool            `json:"disabled"`
	Unavailable    bool            `json:"unavailable"`
	NextRetryAfter string          `json:"next_retry_after"`
	Cooldowns      []WtCPACooldown `json:"cooldowns"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
	Modtime        string          `json:"modtime"`
	Success        int64           `json:"success"`
	Failed         int64           `json:"failed"`
}

type wtCPACredentialPage struct {
	Files   []WtCPACredential `json:"files"`
	HasMore bool              `json:"has_more"`
}

// WtUsageBucket is one quota window of one model group of a donated account.
type WtUsageBucket struct {
	Name              string  `json:"name"`
	Window            string  `json:"window"`
	RemainingFraction float64 `json:"remaining_fraction"`
	ResetAt           int64   `json:"reset_at"`
}

// WtUsageGroup is one model group of Google's quota summary.
type WtUsageGroup struct {
	Name    string          `json:"name"`
	Buckets []WtUsageBucket `json:"buckets"`
}

// WtUsage is what Google says a donated account has left (wind-tide/API.md §14).
type WtUsage struct {
	FetchedAt int64          `json:"fetched_at"`
	Tier      string         `json:"tier"`
	Groups    []WtUsageGroup `json:"groups"`
}

// wtCPADo sends one management request and returns the HTTP status and the
// (capped) body of the answer. ErrWtPoolUnavailable covers a disabled pool,
// an unreachable CPA, a redirect, an oversized body and a wrong key; a 401
// is logged with a hint and never retried, because CPA locks a client out
// after five wrong keys. op names the call in the log.
func wtCPADo(ctx context.Context, op, method, path string, query url.Values, body any) (int, []byte, error) {
	if !WtDonationEnabled() {
		return 0, nil, ErrWtPoolUnavailable
	}
	target := strings.TrimRight(strings.TrimSpace(os.Getenv("WT_CPA_URL")), "/") + wtCPAPrefix + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var payload io.Reader
	if body != nil {
		encoded, err := common.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, payload)
	if err != nil {
		common.SysError(common.LogText("wt donation: cpa %s request could not be built", op))
		return 0, nil, ErrWtPoolUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(os.Getenv("WT_CPA_MANAGEMENT_KEY")))
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := http.Client{}
	if shared := GetHttpClient(); shared != nil {
		client = *shared
	}
	client.Timeout = wtCPATimeout
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
		common.SysError(common.LogText("wt donation: cpa %s %s", op, reason))
		return 0, nil, ErrWtPoolUnavailable
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, wtCPAMaxBody+1))
	if err != nil || len(raw) > wtCPAMaxBody {
		common.SysError(common.LogText("wt donation: cpa %s response could not be read", op))
		return 0, nil, ErrWtPoolUnavailable
	}
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		common.SysError(common.LogText("wt donation: cpa %s failed with status %d; check WT_CPA_MANAGEMENT_KEY before retrying, CPA locks the site out after repeated wrong keys", op, response.StatusCode))
		return response.StatusCode, raw, ErrWtPoolUnavailable
	case response.StatusCode >= 300 && response.StatusCode < 400:
		common.SysError(common.LogText("wt donation: cpa %s failed with status %d", op, response.StatusCode))
		return response.StatusCode, raw, ErrWtPoolUnavailable
	}
	return response.StatusCode, raw, nil
}

// wtCPAUnexpected logs an answer the site cannot use and returns the pool error.
func wtCPAUnexpected(op string, status int) error {
	common.SysError(common.LogText("wt donation: cpa %s failed with status %d", op, status))
	return ErrWtPoolUnavailable
}

// WtCPAAuthURL asks CPA for a Google authorization link and returns it with
// login_hint set to email, plus the state CPA issued for this session.
func WtCPAAuthURL(ctx context.Context, email string) (string, string, error) {
	status, raw, err := wtCPADo(ctx, "auth-url", http.MethodGet, "/oauth/auth-url", url.Values{"provider": {WtDonationProvider}}, nil)
	if err != nil {
		return "", "", err
	}
	var reply struct {
		Status string `json:"status"`
		URL    string `json:"url"`
		State  string `json:"state"`
	}
	if status != http.StatusOK || common.Unmarshal(raw, &reply) != nil || reply.Status != "ok" || reply.State == "" {
		return "", "", wtCPAUnexpected("auth-url", status)
	}
	link, err := url.Parse(reply.URL)
	if err != nil || link.Scheme != "https" || link.Host == "" {
		return "", "", wtCPAUnexpected("auth-url", status)
	}
	values := link.Query()
	values.Set("login_hint", email)
	link.RawQuery = values.Encode()
	return link.String(), reply.State, nil
}

// WtCPASubmitCallback hands the authorization code of a session to CPA, which
// exchanges it for tokens and writes the credential file.
func WtCPASubmitCallback(ctx context.Context, state, code string) error {
	body := map[string]string{"provider": WtDonationProvider, "state": state, "code": code}
	status, raw, err := wtCPADo(ctx, "callback", http.MethodPost, "/oauth/callback", nil, body)
	if err != nil {
		return err
	}
	var reply struct {
		Status string `json:"status"`
	}
	if status < 200 || status >= 300 || common.Unmarshal(raw, &reply) != nil || reply.Status != "ok" {
		return wtCPAUnexpected("callback", status)
	}
	return nil
}

// WtCPAOAuthStatus reports an authorization session: wait, ok, error with
// CPA's reason, or unknown once CPA has forgotten the state.
func WtCPAOAuthStatus(ctx context.Context, state string) (string, string, error) {
	status, raw, err := wtCPADo(ctx, "oauth-status", http.MethodGet, "/oauth/status", url.Values{"state": {state}}, nil)
	if err != nil {
		return "", "", err
	}
	if status >= 400 && status < 500 {
		return WtCPAOAuthUnknown, "", nil
	}
	var reply struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if status != http.StatusOK || common.Unmarshal(raw, &reply) != nil {
		return "", "", wtCPAUnexpected("oauth-status", status)
	}
	switch reply.Status {
	case WtCPAOAuthWait, WtCPAOAuthOK:
		return reply.Status, "", nil
	case WtCPAOAuthError:
		if strings.EqualFold(strings.TrimSpace(reply.Error), wtCPAUnknownStateError) {
			return WtCPAOAuthUnknown, "", nil
		}
		return WtCPAOAuthError, reply.Error, nil
	}
	return WtCPAOAuthUnknown, "", nil
}

// WtCPAGetCredential looks a credential up by its exact file name; nil when
// the pool has none.
func WtCPAGetCredential(ctx context.Context, name string) (*WtCPACredential, error) {
	status, raw, err := wtCPADo(ctx, "credential", http.MethodGet, "/credentials", url.Values{"name": {name}}, nil)
	if err != nil {
		return nil, err
	}
	var page wtCPACredentialPage
	if status != http.StatusOK || common.Unmarshal(raw, &page) != nil {
		return nil, wtCPAUnexpected("credential", status)
	}
	for i := range page.Files {
		if page.Files[i].Name == name {
			return &page.Files[i], nil
		}
	}
	return nil, nil
}

// WtCPAListCredentials lists the whole pool, at most wtCPAListMaxPages pages
// of wtCPAListPageSize entries.
func WtCPAListCredentials(ctx context.Context) ([]WtCPACredential, error) {
	all := []WtCPACredential{}
	for page := 1; page <= wtCPAListMaxPages; page++ {
		query := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(wtCPAListPageSize)}}
		status, raw, err := wtCPADo(ctx, "credentials", http.MethodGet, "/credentials", query, nil)
		if err != nil {
			return nil, err
		}
		var reply wtCPACredentialPage
		if status != http.StatusOK || common.Unmarshal(raw, &reply) != nil {
			return nil, wtCPAUnexpected("credentials", status)
		}
		all = append(all, reply.Files...)
		if !reply.HasMore || len(reply.Files) == 0 {
			break
		}
	}
	return all, nil
}

// WtCPASetCredentialNote labels a credential in CPA's console, best effort.
func WtCPASetCredentialNote(ctx context.Context, name, note string) error {
	status, _, err := wtCPADo(ctx, "note", http.MethodPatch, "/credentials/fields", nil, map[string]string{"name": name, "note": note})
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return wtCPAUnexpected("note", status)
	}
	return nil
}

// WtCPADeleteCredential removes a credential file; a file CPA no longer has
// counts as removed.
func WtCPADeleteCredential(ctx context.Context, name string) error {
	status, _, err := wtCPADo(ctx, "delete", http.MethodDelete, "/credentials", url.Values{"name": {name}}, nil)
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusNotFound {
		return nil
	}
	return wtCPAUnexpected("delete", status)
}

// wtCPAAPICall makes CPA send one POST to Google with the access token of the
// credential at authIndex ($TOKEN$ is replaced by CPA, which refreshes the
// token first). It returns Google's status and body.
func wtCPAAPICall(ctx context.Context, authIndex, target, data string) (int, []byte, error) {
	body := map[string]any{
		"authIndex": authIndex,
		"method":    http.MethodPost,
		"url":       target,
		"header": map[string]string{
			"Authorization": "Bearer $TOKEN$",
			"Content-Type":  "application/json",
			"User-Agent":    wtGoogleUserAgent,
		},
		"data": data,
	}
	status, raw, err := wtCPADo(ctx, "api-call", http.MethodPost, "/requests/api-call", nil, body)
	if err != nil {
		return 0, nil, err
	}
	var reply struct {
		StatusCode int             `json:"status_code"`
		Body       json.RawMessage `json:"body"`
	}
	if status != http.StatusOK || common.Unmarshal(raw, &reply) != nil {
		return 0, nil, wtCPAUnexpected("api-call", status)
	}
	// CPA wraps Google's body as a JSON string; accept raw JSON as well.
	var text string
	if common.Unmarshal(reply.Body, &text) == nil {
		return reply.StatusCode, []byte(text), nil
	}
	return reply.StatusCode, reply.Body, nil
}

// WtCPAFetchUsage asks Google, through CPA, how much of its allowance a
// donated account has left, and which tier it is on. The string classifies
// a failure for the member; it never carries a token or a body.
func WtCPAFetchUsage(ctx context.Context, authIndex, projectID string) (*WtUsage, string, error) {
	data, err := common.Marshal(map[string]string{"project": projectID})
	if err != nil {
		return nil, "", err
	}
	status, body, err := wtCPAAPICall(ctx, authIndex, wtGoogleQuotaHost+wtGoogleQuotaPath, string(data))
	if err == nil && (status == http.StatusNotFound || status >= 500) {
		status, body, err = wtCPAAPICall(ctx, authIndex, wtGoogleQuotaFallback+wtGoogleQuotaPath, string(data))
	}
	if err != nil {
		return nil, "Account pool unreachable", err
	}
	if status != http.StatusOK {
		return nil, fmt.Sprintf("HTTP %d from Google", status), ErrWtUsageUnavailable
	}
	usage, err := parseWtUsage(body)
	if err != nil {
		return nil, "Unreadable usage response from Google", ErrWtUsageUnavailable
	}
	usage.FetchedAt = model.WtNow().Unix()
	usage.Tier = wtFetchTier(ctx, authIndex)
	return usage, "", nil
}

// parseWtUsage reads Google's quota summary. Buckets without a numeric
// remaining fraction are dropped; both camelCase and snake_case field names
// are accepted.
func parseWtUsage(body []byte) (*WtUsage, error) {
	var reply struct {
		Groups []struct {
			DisplayName  string `json:"displayName"`
			DisplayName2 string `json:"display_name"`
			Name         string `json:"name"`
			Buckets      []struct {
				DisplayName  string   `json:"displayName"`
				DisplayName2 string   `json:"display_name"`
				Window       string   `json:"window"`
				Remaining    *float64 `json:"remainingFraction"`
				Remaining2   *float64 `json:"remaining_fraction"`
				ResetTime    string   `json:"resetTime"`
				ResetTime2   string   `json:"reset_time"`
			} `json:"buckets"`
		} `json:"groups"`
	}
	if err := common.Unmarshal(body, &reply); err != nil {
		return nil, err
	}
	usage := &WtUsage{Groups: []WtUsageGroup{}}
	for _, group := range reply.Groups {
		name := cmp.Or(group.DisplayName, group.DisplayName2, group.Name)
		buckets := []WtUsageBucket{}
		for _, bucket := range group.Buckets {
			remaining := bucket.Remaining
			if remaining == nil {
				remaining = bucket.Remaining2
			}
			if remaining == nil {
				continue
			}
			buckets = append(buckets, WtUsageBucket{
				Name:              cmp.Or(bucket.DisplayName, bucket.DisplayName2, name),
				Window:            bucket.Window,
				RemainingFraction: min(max(*remaining, 0), 1),
				ResetAt:           wtParseTime(cmp.Or(bucket.ResetTime, bucket.ResetTime2)),
			})
		}
		usage.Groups = append(usage.Groups, WtUsageGroup{Name: name, Buckets: buckets})
	}
	return usage, nil
}

// wtFetchTier asks Google which tier the account is on; any failure is
// "unknown" and does not affect the usage result.
func wtFetchTier(ctx context.Context, authIndex string) string {
	status, body, err := wtCPAAPICall(ctx, authIndex, wtGoogleQuotaHost+wtGoogleTierPath, `{"metadata":{"ideType":"ANTIGRAVITY"}}`)
	if err != nil || status != http.StatusOK {
		return "unknown"
	}
	var reply struct {
		PaidTier struct {
			ID string `json:"id"`
		} `json:"paidTier"`
		CurrentTier struct {
			ID string `json:"id"`
		} `json:"currentTier"`
	}
	if common.Unmarshal(body, &reply) != nil {
		return "unknown"
	}
	return WtTierName(cmp.Or(reply.PaidTier.ID, reply.CurrentTier.ID))
}

// WtTierName maps Google's tier IDs to the short names the console shows.
func WtTierName(id string) string {
	switch id {
	case "free-tier":
		return "free"
	case "g1-pro-tier":
		return "pro"
	case "g1-ultra-tier":
		return "ultra"
	case "g1-ultra-lite-tier":
		return "ultra_lite"
	}
	return "unknown"
}

// wtParseTime turns an RFC 3339 time into Unix seconds, 0 when absent or
// unreadable.
func wtParseTime(raw string) int64 {
	at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	return at.Unix()
}
