package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

// wind-tide: the live side of donated accounts (wind-tide/API.md §14; design
// in wind-tide/specs/2026-10-17-antigravity-donation-design.md §7.3–§7.5):
// what a CPA credential entry means to the member, the two short caches, and
// the verification of submitted donations.

// The pool states of an active donation.
const (
	WtPoolOK           = "ok"
	WtPoolCooldown     = "cooldown"
	WtPoolTokenInvalid = "token_invalid"
	WtPoolMissing      = "missing"
	WtPoolDisabled     = "disabled"
	WtPoolUnknown      = "unknown"
)

const (
	wtCredentialsCacheTTL = 15 * time.Second
	wtUsageCacheTTL       = 60 * time.Second
	wtPoolMessageMax      = 200
)

// wtTokenInvalidHints are the CPA status messages that mean the refresh token
// no longer works, whatever else is going on.
var wtTokenInvalidHints = []string{"unauthori", "invalid_grant", "invalid token", "refresh token", "401"}

var (
	wtCredentialsMu       sync.Mutex
	wtCredentialsCache    []WtCPACredential
	wtCredentialsCachedAt time.Time

	wtUsageMu    sync.Mutex
	wtUsageCache = map[int]wtUsageEntry{}
)

type wtUsageEntry struct {
	usage    *WtUsage
	cachedAt time.Time
}

// WtPoolInfo is what the list shows about an active donation's credential.
type WtPoolInfo struct {
	State   string `json:"state"`
	Message string `json:"message"`
	RetryAt int64  `json:"retry_at"`
	Success int64  `json:"success"`
	Failed  int64  `json:"failed"`
}

// WtPoolStateOf maps a CPA credential entry to the state the member sees; a
// nil entry means the pool no longer has the file. Token keywords come before
// recovery times because CPA also sets status "error" during a quota
// cooldown, and only a recovery time tells the two apart.
func WtPoolStateOf(entry *WtCPACredential, now time.Time) WtPoolInfo {
	if entry == nil {
		return WtPoolInfo{State: WtPoolMissing}
	}
	message := entry.StatusMessage
	if utf8.RuneCountInString(message) > wtPoolMessageMax {
		message = string([]rune(message)[:wtPoolMessageMax])
	}
	info := WtPoolInfo{Message: message, Success: entry.Success, Failed: entry.Failed}
	lowered := strings.ToLower(entry.StatusMessage)
	retryAt := wtLatestRetry(entry, now)
	switch {
	case entry.Disabled:
		info.State = WtPoolDisabled
	case slices.ContainsFunc(wtTokenInvalidHints, func(hint string) bool { return strings.Contains(lowered, hint) }):
		info.State = WtPoolTokenInvalid
	case retryAt > 0 || entry.Unavailable:
		info.State = WtPoolCooldown
		info.RetryAt = retryAt
	case entry.Status == "error":
		info.State = WtPoolTokenInvalid
	default:
		info.State = WtPoolOK
	}
	return info
}

// wtLatestRetry is the latest recovery time of a credential that is still in
// the future, as Unix seconds; 0 when no timer is running.
func wtLatestRetry(entry *WtCPACredential, now time.Time) int64 {
	raws := make([]string, 0, len(entry.Cooldowns)+1)
	raws = append(raws, entry.NextRetryAfter)
	for _, cooldown := range entry.Cooldowns {
		raws = append(raws, cooldown.RetryAt)
	}
	var latest int64
	for _, raw := range raws {
		at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
		if err == nil && at.After(now) {
			latest = max(latest, at.Unix())
		}
	}
	return latest
}

// WtPoolSnapshot digests the file names of a credential list, so a donation
// can later tell a new file from everybody else's without storing their
// addresses.
func WtPoolSnapshot(list []WtCPACredential) []string {
	names := make([]string, 0, len(list))
	for _, entry := range list {
		names = append(names, wtNameDigest(entry.Name))
	}
	return names
}

func wtNameDigest(name string) string {
	digest := sha256.Sum256([]byte(name))
	return hex.EncodeToString(digest[:])
}

// WtPoolCredentials is the pool's credential list, at most 15 seconds old.
// The lock is held through the request so that one list call serves every
// member who opens the page at the same time.
func WtPoolCredentials(ctx context.Context) ([]WtCPACredential, error) {
	wtCredentialsMu.Lock()
	defer wtCredentialsMu.Unlock()
	if wtCredentialsCache != nil && time.Since(wtCredentialsCachedAt) < wtCredentialsCacheTTL {
		return wtCredentialsCache, nil
	}
	list, err := WtCPAListCredentials(ctx)
	if err != nil {
		return nil, err
	}
	wtCredentialsCache, wtCredentialsCachedAt = list, time.Now()
	return list, nil
}

// WtForgetPoolCredentials drops the list after the pool changed.
func WtForgetPoolCredentials() {
	wtCredentialsMu.Lock()
	wtCredentialsCache = nil
	wtCredentialsMu.Unlock()
}

// WtDonationUsage is a donation's remaining allowance, from the 60-second
// cache unless fresh is set; a fresh answer replaces the cached one. The
// string classifies a failure for the member.
func WtDonationUsage(ctx context.Context, donation *model.WtDonation, fresh bool) (*WtUsage, string, error) {
	if !fresh {
		wtUsageMu.Lock()
		entry, cached := wtUsageCache[donation.Id]
		wtUsageMu.Unlock()
		if cached && time.Since(entry.cachedAt) < wtUsageCacheTTL {
			return entry.usage, "", nil
		}
	}
	if donation.AuthIndex == "" || donation.ProjectId == "" {
		return nil, "The credential has no Google project", ErrWtUsageUnavailable
	}
	usage, detail, err := WtCPAFetchUsage(ctx, donation.AuthIndex, donation.ProjectId)
	if err != nil {
		return nil, detail, err
	}
	wtUsageMu.Lock()
	wtUsageCache[donation.Id] = wtUsageEntry{usage: usage, cachedAt: time.Now()}
	wtUsageMu.Unlock()
	return usage, "", nil
}

// WtForgetDonationUsage drops the cached usage of a removed donation.
func WtForgetDonationUsage(id int) {
	wtUsageMu.Lock()
	delete(wtUsageCache, id)
	wtUsageMu.Unlock()
}

// WtResetDonationCaches empties both caches; tests call it between cases.
func WtResetDonationCaches() {
	WtForgetPoolCredentials()
	wtUsageMu.Lock()
	clear(wtUsageCache)
	wtUsageMu.Unlock()
}

// The reasons a verification can end with (wind-tide/API.md §14).
const (
	WtDonationFailTimeout   = "Authorization timed out, start again"
	WtDonationFailDenied    = "Google authorization failed, start again"
	WtDonationFailUnknown   = "Could not confirm the donation result, start again"
	WtDonationFailMismatch  = "The authorized Google account does not match the email you entered"
	WtDonationFailDuplicate = "This account is already in the pool"
	WtDonationFailPool      = "The account pool is temporarily unavailable, try again later"
)

// wtDonationFreshness is how much older than the start of a donation a
// credential file may be and still count as written by it.
const wtDonationFreshness = 5 * time.Second

// Tests shorten the polling and make verification run inline.
var (
	WtDonationVerifyInterval = 2 * time.Second
	WtDonationVerifyDeadline = 120 * time.Second
	WtDonationVerifySync     bool
)

var (
	wtDonationVerifyOnce sync.Once
	// wtDonationInflight holds the IDs of the donations being verified.
	wtDonationInflight sync.Map
)

// WtDonationVerifyJob is one submitted callback to verify.
type WtDonationVerifyJob struct {
	DonationID int
	UserID     int
	Email      string
	State      string
	StartedAt  int64
	// PoolBefore holds the digests of the file names the pool had when the
	// member started (WtPoolSnapshot); empty after a restart, when no
	// fallback matching is done.
	PoolBefore map[string]struct{}
}

// WtStartDonationVerification polls CPA for the outcome of a job and settles
// the donation, in the background unless WtDonationVerifySync is set. A job
// already running for the same donation is left alone.
func WtStartDonationVerification(job WtDonationVerifyJob) {
	if _, running := wtDonationInflight.LoadOrStore(job.DonationID, struct{}{}); running {
		return
	}
	run := func() {
		defer wtDonationInflight.Delete(job.DonationID)
		wtVerifyDonation(job)
	}
	if WtDonationVerifySync {
		run()
		return
	}
	gopool.Go(run)
}

// wtVerifyDonation asks CPA every WtDonationVerifyInterval how the
// authorization went, until it is settled or WtDonationVerifyDeadline has
// passed. An unreachable CPA is retried until the deadline.
func wtVerifyDonation(job WtDonationVerifyJob) {
	ctx := context.Background()
	deadline := time.Now().Add(WtDonationVerifyDeadline)
	for {
		status, detail, err := WtCPAOAuthStatus(ctx, job.State)
		switch {
		case err == nil && status == WtCPAOAuthOK:
			wtFinishDonation(ctx, job, true)
			return
		case err == nil && status == WtCPAOAuthUnknown:
			wtFinishDonation(ctx, job, false)
			return
		case err == nil && status == WtCPAOAuthError:
			reason := WtDonationFailDenied
			if strings.Contains(strings.ToLower(detail), "timed out") {
				reason = WtDonationFailTimeout
			}
			// CPA's reason is a fixed English phrase without secrets.
			common.SysError(common.LogText("wt donation: donation %d of member %d failed at google: %s", job.DonationID, job.UserID, detail))
			wtFailDonation(job, reason)
			return
		}
		if time.Now().After(deadline) {
			wtFailDonation(job, WtDonationFailTimeout)
			return
		}
		time.Sleep(WtDonationVerifyInterval)
	}
}

// wtFinishDonation looks for the credential a job produced and binds it, or
// fails the donation. confirmed says CPA reported the authorization done;
// otherwise CPA had already forgotten the session.
func wtFinishDonation(ctx context.Context, job WtDonationVerifyJob, confirmed bool) {
	entry, err := wtFindCredentialByEmail(ctx, job)
	if err == nil && entry == nil && len(job.PoolBefore) > 0 {
		entry, err = wtFindNewCredential(ctx, job)
	}
	if err != nil {
		wtFailDonation(job, WtDonationFailPool)
		return
	}
	if entry == nil {
		if confirmed {
			wtFailDonation(job, WtDonationFailMismatch)
		} else {
			wtFailDonation(job, WtDonationFailUnknown)
		}
		return
	}
	claimed, err := model.WtDonationCredentialClaimed(entry.Name, job.DonationID)
	if err != nil {
		common.SysError(common.LogText("wt donation: donation %d could not be checked for conflicts: %s", job.DonationID, err.Error()))
		return
	}
	if claimed {
		wtFailDonation(job, WtDonationFailDuplicate)
		return
	}
	email := strings.ToLower(strings.TrimSpace(entry.Email))
	if email == "" {
		email = job.Email
	}
	if err := model.BindWtDonation(job.DonationID, entry.Name, entry.AuthIndex, entry.ProjectID, email); err != nil {
		common.SysError(common.LogText("wt donation: donation %d could not be marked active: %s", job.DonationID, err.Error()))
		return
	}
	WtForgetPoolCredentials()
	if err := WtCPASetCredentialNote(ctx, entry.Name, fmt.Sprintf("donor:%d", job.UserID)); err != nil {
		common.SysError(common.LogText("wt donation: the note of donation %d was not saved", job.DonationID))
	}
	model.RecordLog(job.UserID, model.LogTypeSystem, common.NewMessage("Donated account #{{id}} joined the pool", map[string]any{"id": job.DonationID}))
}

// wtFindCredentialByEmail finds the credential of the job's email: by exact
// file name first, then case-insensitively in the list. A file older than
// the job is not the donation.
func wtFindCredentialByEmail(ctx context.Context, job WtDonationVerifyJob) (*WtCPACredential, error) {
	name := WtCredentialName(job.Email)
	entry, err := WtCPAGetCredential(ctx, name)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		list, err := WtCPAListCredentials(ctx)
		if err != nil {
			return nil, err
		}
		for i := range list {
			if strings.EqualFold(list[i].Email, job.Email) || strings.EqualFold(list[i].Name, name) {
				entry = &list[i]
				break
			}
		}
	}
	if entry == nil || !wtCredentialFresh(entry, job.StartedAt) {
		return nil, nil
	}
	return entry, nil
}

// wtCredentialFresh accepts an entry written after the job started; an entry
// without a readable time counts as fresh, since the pool had no such file
// when the member started.
func wtCredentialFresh(entry *WtCPACredential, startedAt int64) bool {
	raw := strings.TrimSpace(entry.UpdatedAt)
	if raw == "" {
		raw = strings.TrimSpace(entry.Modtime)
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return true
	}
	return !at.Before(time.Unix(startedAt, 0).Add(-wtDonationFreshness))
}

// wtFindNewCredential is the fallback when the member authorized another
// account than the one typed: the single Antigravity credential that is new
// since the snapshot, fresh, and claimed by no other donation. Two or more
// candidates cannot be told apart.
func wtFindNewCredential(ctx context.Context, job WtDonationVerifyJob) (*WtCPACredential, error) {
	list, err := WtCPAListCredentials(ctx)
	if err != nil {
		return nil, err
	}
	var found *WtCPACredential
	for i := range list {
		entry := &list[i]
		if entry.Provider != WtDonationProvider || !wtCredentialFresh(entry, job.StartedAt) {
			continue
		}
		if _, known := job.PoolBefore[wtNameDigest(entry.Name)]; known {
			continue
		}
		claimed, err := model.WtDonationCredentialClaimed(entry.Name, job.DonationID)
		if err != nil {
			return nil, err
		}
		if claimed {
			continue
		}
		if found != nil {
			return nil, nil
		}
		found = entry
	}
	return found, nil
}

// wtFailDonation records the reason and tells the member.
func wtFailDonation(job WtDonationVerifyJob, reason string) {
	if err := model.FailWtDonation(job.DonationID, reason); err != nil {
		common.SysError(common.LogText("wt donation: donation %d could not be marked failed: %s", job.DonationID, err.Error()))
		return
	}
	model.RecordLog(job.UserID, model.LogTypeSystem, common.NewMessage("Donation #{{id}} failed", map[string]any{"id": job.DonationID}))
}

// StartWtDonationVerifyTask settles, once per process and only on the master
// node, the donations a restart left verifying. It does not poll: by now the
// pool either has their credential or it never will.
func StartWtDonationVerifyTask() {
	wtDonationVerifyOnce.Do(func() {
		if !common.IsMasterNode || !WtDonationEnabled() {
			return
		}
		gopool.Go(WtSettleVerifyingDonations)
	})
}

// WtSettleVerifyingDonations is the pass StartWtDonationVerifyTask makes:
// every verifying donation is matched by address, with no fallback, since
// the snapshot of the pool did not survive the restart.
func WtSettleVerifyingDonations() {
	donations, err := model.ListVerifyingWtDonations()
	if err != nil {
		common.SysError(common.LogText("wt donation: verifying donations could not be listed: %s", err.Error()))
		return
	}
	ctx := context.Background()
	for _, donation := range donations {
		if _, running := wtDonationInflight.LoadOrStore(donation.Id, struct{}{}); running {
			continue
		}
		wtFinishDonation(ctx, WtDonationVerifyJob{DonationID: donation.Id, UserID: donation.UserId, Email: donation.Email, StartedAt: donation.StartedAt}, false)
		wtDonationInflight.Delete(donation.Id)
	}
}
