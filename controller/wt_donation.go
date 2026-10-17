package controller

import (
	"cmp"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// wind-tide: donated Antigravity accounts (wind-tide/API.md §14). A member
// donates through CPA's Google authorization; the site keeps the ledger and
// never sees a token.

const (
	wtDonationFlowTTL      = 5 * time.Minute
	wtDonationCallbackMax  = 2048
	wtDonationTestInterval = 60 * time.Second
	wtDonationTestsPerDay  = 20
	wtDonationRemoveGrace  = 10 * time.Minute
)

var (
	errWtDonationDisabled     = common.NewMessage("Account donation is not enabled on this site")
	errWtDonationSession      = common.NewMessage("This donation session is invalid or has expired, start again")
	errWtDonationCallback     = common.NewMessage("Paste the complete address from the browser's address bar")
	errWtDonationForeignState = common.NewMessage("This address does not belong to the current donation")
	errWtDonationDenied       = common.NewMessage("Google denied the authorization")
	errWtDonationVerifying    = common.NewMessage("This donation is still being verified")
	errWtDonationNotActive    = common.NewMessage("Only an active donation can be tested")
	errWtDonationTestSoon     = common.NewMessage("Wait a minute before testing again")
	errWtDonationTestLimit    = common.NewMessage("This account has reached today's test limit")
)

// wtDonationFlowPayload is the server-side state of one authorization, kept
// in auth_flows and never sent to the browser. PoolBefore digests the file
// names the pool had when the member started (service.WtPoolSnapshot).
type wtDonationFlowPayload struct {
	CPAState   string   `json:"cpa_state"`
	Email      string   `json:"email"`
	DonationID int      `json:"donation_id"`
	StartedAt  int64    `json:"started_at"`
	PoolBefore []string `json:"pool_before"`
}

// wtDonationReady answers the disabled message unless the site is wired to a pool.
func wtDonationReady(c *gin.Context) bool {
	if service.WtDonationEnabled() {
		return true
	}
	common.ApiError(c, errWtDonationDisabled)
	return false
}

// wtDonationOf loads the donation named by :id for the signed-in member.
func wtDonationOf(c *gin.Context) (*model.WtDonation, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, model.ErrWtDonationNotFound)
		return nil, false
	}
	donation, err := model.GetWtDonation(id, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	return donation, true
}

// WtStartDonation begins a donation, or the re-authorization of one: it
// checks the limits, snapshots the pool and hands out CPA's Google link.
func WtStartDonation(c *gin.Context) {
	if !wtDonationReady(c) {
		return
	}
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		common.ApiError(c, errWtDonationSession)
		return
	}
	var input struct {
		Email      string `json:"email"`
		DonationID int    `json:"donation_id"`
	}
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	ctx := c.Request.Context()
	var email string
	if input.DonationID != 0 {
		// Re-authorization: the record's address wins over the body, and the
		// account is expected to be in the pool already.
		donation, err := model.GetWtDonation(input.DonationID, identity.UserID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if donation.Status != model.WtDonationActive && donation.Status != model.WtDonationFailed {
			common.ApiError(c, model.ErrWtDonationNotFound)
			return
		}
		email = donation.Email
	} else {
		normalized, valid := model.WtNormalizeDonationEmail(input.Email)
		if !valid {
			common.ApiError(c, model.ErrWtDonationEmail)
			return
		}
		email = normalized
		slots, err := model.CountWtDonationSlots(identity.UserID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if slots >= model.WtDonationMaxPerUser {
			common.ApiError(c, model.ErrWtDonationLimit)
			return
		}
		inPool, err := model.WtDonationEmailInPool(email)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if inPool {
			common.ApiError(c, model.ErrWtDonationDuplicate)
			return
		}
	}
	pool, err := service.WtCPAListCredentials(ctx)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if input.DonationID == 0 {
		existing, err := service.WtCPAGetCredential(ctx, service.WtCredentialName(email))
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if existing != nil || slices.ContainsFunc(pool, func(entry service.WtCPACredential) bool {
			return strings.EqualFold(strings.TrimSpace(entry.Email), email)
		}) {
			common.ApiError(c, model.ErrWtDonationDuplicate)
			return
		}
	}
	authURL, state, err := service.WtCPAAuthURL(ctx, email)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	payload, err := common.Marshal(wtDonationFlowPayload{
		CPAState:   state,
		Email:      email,
		DonationID: input.DonationID,
		StartedAt:  model.WtNow().Unix(),
		PoolBefore: service.WtPoolSnapshot(pool),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiresAt := time.Now().Add(wtDonationFlowTTL)
	token, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose:   model.AuthFlowPurposeWtDonation,
		Provider:  service.WtDonationProvider,
		Intent:    model.AuthFlowIntentWtDonate,
		UserId:    identity.UserID,
		SessionId: identity.SessionID,
		Payload:   string(payload),
		ExpiresAt: expiresAt,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"flow_token":        token,
		"authorization_url": authURL,
		"expires_at":        expiresAt.Unix(),
		"email":             email,
	})
}

// WtSubmitDonationCallback takes the address Google sent the member to,
// forwards only its state and code to CPA and starts the verification. The
// address itself is parsed, never requested.
func WtSubmitDonationCallback(c *gin.Context) {
	if !wtDonationReady(c) {
		return
	}
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		common.ApiError(c, errWtDonationSession)
		return
	}
	var input struct {
		FlowToken   string `json:"flow_token"`
		CallbackURL string `json:"callback_url"`
	}
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	match := model.AuthFlowMatch{Purpose: model.AuthFlowPurposeWtDonation, UserId: identity.UserID, SessionId: identity.SessionID}
	flow, err := model.GetAuthFlow(input.FlowToken, match)
	if err != nil {
		common.ApiError(c, errWtDonationSession)
		return
	}
	var payload wtDonationFlowPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
		common.ApiError(c, errWtDonationSession)
		return
	}
	callback := strings.TrimSpace(input.CallbackURL)
	parsed, err := url.Parse(callback)
	if callback == "" || len(callback) > wtDonationCallbackMax || err != nil {
		common.ApiError(c, errWtDonationCallback)
		return
	}
	query := parsed.Query()
	state, code := query.Get("state"), query.Get("code")
	if state == "" {
		common.ApiError(c, errWtDonationCallback)
		return
	}
	audit := map[string]any{"id": payload.DonationID, "provider": service.WtDonationProvider, "success": false}
	if state != payload.CPAState {
		// A wrong paste, or somebody else's address: the flow stays usable.
		common.SysError(common.LogText("wt donation: member %d pasted the address of another authorization", identity.UserID))
		recordUserSecurityAudit(c, identity.UserID, "wt.donation_add", audit)
		common.ApiError(c, errWtDonationForeignState)
		return
	}
	if query.Get("error") != "" {
		if _, err := model.ConsumeAuthFlow(input.FlowToken, match); err != nil {
			common.ApiError(c, errWtDonationSession)
			return
		}
		recordUserSecurityAudit(c, identity.UserID, "wt.donation_add", audit)
		common.ApiError(c, errWtDonationDenied)
		return
	}
	if code == "" {
		common.ApiError(c, errWtDonationCallback)
		return
	}
	donation := &model.WtDonation{Id: payload.DonationID, UserId: identity.UserID, Provider: service.WtDonationProvider, Email: payload.Email}
	_, err = model.ConsumeAuthFlowWithAction(input.FlowToken, match, func(tx *gorm.DB, _ *model.AuthFlow) error {
		return model.StartWtDonationVerification(tx, donation, payload.CPAState, payload.StartedAt)
	})
	if err != nil {
		if errors.Is(err, model.ErrWtDonationNotFound) {
			common.ApiError(c, err)
			return
		}
		common.ApiError(c, errWtDonationSession)
		return
	}
	audit["id"] = donation.Id
	// The CPA call stays outside the transaction: it may take 15 seconds and
	// must not hold SQLite's write lock. A failure leaves a failed donation.
	if err := service.WtCPASubmitCallback(c.Request.Context(), payload.CPAState, code); err != nil {
		if failErr := model.FailWtDonation(donation.Id, service.WtDonationFailPool); failErr != nil {
			common.SysError(common.LogText("wt donation: donation %d could not be marked failed: %s", donation.Id, failErr.Error()))
		}
		recordUserSecurityAudit(c, identity.UserID, "wt.donation_add", audit)
		common.ApiError(c, err)
		return
	}
	poolBefore := make(map[string]struct{}, len(payload.PoolBefore))
	for _, digest := range payload.PoolBefore {
		poolBefore[digest] = struct{}{}
	}
	service.WtStartDonationVerification(service.WtDonationVerifyJob{
		DonationID: donation.Id,
		UserID:     identity.UserID,
		Email:      payload.Email,
		State:      payload.CPAState,
		StartedAt:  payload.StartedAt,
		PoolBefore: poolBefore,
	})
	audit["success"] = true
	recordUserSecurityAudit(c, identity.UserID, "wt.donation_add", audit)
	common.ApiSuccess(c, gin.H{"id": donation.Id, "status": model.WtDonationVerifying})
}

// wtDonationItem is one row of the member's list (wind-tide/API.md §14).
// pool is the credential list by file name; nil means it could not be
// fetched, which shows as unknown rather than as a missing file.
func wtDonationItem(donation *model.WtDonation, pool map[string]*service.WtCPACredential, now time.Time) gin.H {
	item := gin.H{
		"id":          donation.Id,
		"email":       donation.Email,
		"status":      donation.Status,
		"fail_reason": donation.FailReason,
		"pool":        nil,
		"last_test":   nil,
		"created_at":  donation.CreatedAt,
		"verified_at": donation.VerifiedAt,
	}
	if donation.Status == model.WtDonationActive {
		if pool == nil {
			item["pool"] = service.WtPoolInfo{State: service.WtPoolUnknown}
		} else {
			item["pool"] = service.WtPoolStateOf(pool[donation.CredentialName], now)
		}
	}
	if donation.LastTestAt != 0 {
		item["last_test"] = gin.H{
			"at":         donation.LastTestAt,
			"ok":         donation.LastTestOk == 1,
			"latency_ms": donation.LastTestLatencyMs,
			"message":    donation.LastTestMessage,
		}
	}
	return item
}

// WtListDonations is the member's donations, newest first, with the live
// state of the active ones from one cached credential list.
func WtListDonations(c *gin.Context) {
	if !wtDonationReady(c) {
		return
	}
	donations, err := model.ListWtDonations(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var pool map[string]*service.WtCPACredential
	anyActive := slices.ContainsFunc(donations, func(donation *model.WtDonation) bool {
		return donation.Status == model.WtDonationActive
	})
	if anyActive {
		if list, err := service.WtPoolCredentials(c.Request.Context()); err == nil {
			pool = make(map[string]*service.WtCPACredential, len(list))
			for i := range list {
				pool[list[i].Name] = &list[i]
			}
		}
	}
	now := model.WtNow()
	items := make([]gin.H, 0, len(donations))
	for _, donation := range donations {
		items = append(items, wtDonationItem(donation, pool, now))
	}
	common.ApiSuccess(c, gin.H{"max": model.WtDonationMaxPerUser, "items": items})
}

// WtGetDonation is the progress of one donation; the dialog polls it while
// the verification runs. It reads the database only.
func WtGetDonation(c *gin.Context) {
	if !wtDonationReady(c) {
		return
	}
	donation, ok := wtDonationOf(c)
	if !ok {
		return
	}
	common.ApiSuccess(c, gin.H{
		"id":          donation.Id,
		"email":       donation.Email,
		"status":      donation.Status,
		"fail_reason": donation.FailReason,
	})
}

// WtGetDonationUsage is the remaining allowance of an active donation, at
// most a minute old.
func WtGetDonationUsage(c *gin.Context) {
	if !wtDonationReady(c) {
		return
	}
	donation, ok := wtDonationOf(c)
	if !ok {
		return
	}
	if donation.Status != model.WtDonationActive {
		common.ApiError(c, service.ErrWtUsageUnavailable)
		return
	}
	usage, _, err := service.WtDonationUsage(c.Request.Context(), donation, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, usage)
}

// WtTestDonation asks Google for the account's allowance afresh, records the
// attempt with its latency, and holds each account to one test a minute and
// twenty a Beijing day. Google failing is a recorded failure, not an error.
func WtTestDonation(c *gin.Context) {
	if !wtDonationReady(c) {
		return
	}
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		common.ApiError(c, errWtDonationSession)
		return
	}
	donation, ok := wtDonationOf(c)
	if !ok {
		return
	}
	if donation.Status != model.WtDonationActive {
		common.ApiError(c, errWtDonationNotActive)
		return
	}
	now := model.WtNow()
	if donation.LastTestAt != 0 && now.Unix()-donation.LastTestAt < int64(wtDonationTestInterval/time.Second) {
		common.ApiError(c, errWtDonationTestSoon)
		return
	}
	day := model.WtCheckinDay(now)
	testsToday := 0
	if donation.TestsDay == day {
		testsToday = donation.TestsToday
	}
	if testsToday >= wtDonationTestsPerDay {
		common.ApiError(c, errWtDonationTestLimit)
		return
	}
	began := time.Now()
	usage, detail, err := service.WtDonationUsage(c.Request.Context(), donation, true)
	latency := int(time.Since(began).Milliseconds())
	message := ""
	if err != nil {
		message = cmp.Or(detail, err.Error())
	}
	if recordErr := model.RecordWtDonationTest(donation.Id, err == nil, latency, message, day, testsToday+1); recordErr != nil {
		common.ApiError(c, recordErr)
		return
	}
	recordUserSecurityAudit(c, identity.UserID, "wt.donation_test", map[string]any{
		"id":         donation.Id,
		"provider":   service.WtDonationProvider,
		"success":    err == nil,
		"latency_ms": latency,
	})
	common.ApiSuccess(c, gin.H{"ok": err == nil, "latency_ms": latency, "message": message, "usage": usage})
}

// WtRemoveDonation takes the account out of the pool and closes the
// donation. One submitted less than ten minutes ago is left to the verifier,
// so the two do not race; the record stays when the pool could not delete
// the file.
func WtRemoveDonation(c *gin.Context) {
	if !wtDonationReady(c) {
		return
	}
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		common.ApiError(c, errWtDonationSession)
		return
	}
	donation, ok := wtDonationOf(c)
	if !ok {
		return
	}
	if donation.Status == model.WtDonationRemoved {
		common.ApiError(c, model.ErrWtDonationNotFound)
		return
	}
	if donation.Status == model.WtDonationVerifying && model.WtNow().Unix()-donation.SubmittedAt < int64(wtDonationRemoveGrace/time.Second) {
		common.ApiError(c, errWtDonationVerifying)
		return
	}
	audit := map[string]any{"id": donation.Id, "provider": service.WtDonationProvider, "success": false}
	if donation.CredentialName != "" {
		if err := service.WtCPADeleteCredential(c.Request.Context(), donation.CredentialName); err != nil {
			recordUserSecurityAudit(c, identity.UserID, "wt.donation_remove", audit)
			common.ApiError(c, err)
			return
		}
	}
	if err := model.RemoveWtDonation(donation.Id); err != nil {
		common.ApiError(c, err)
		return
	}
	service.WtForgetDonationUsage(donation.Id)
	service.WtForgetPoolCredentials()
	audit["success"] = true
	recordUserSecurityAudit(c, identity.UserID, "wt.donation_remove", audit)
	common.ApiSuccessT(c, "Donation removed", nil)
}
