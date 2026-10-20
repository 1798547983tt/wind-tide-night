package controller

import (
	"cmp"
	"errors"
	"net/http"
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

// wind-tide: donated accounts. A member donates an Antigravity account
// through CPA's Google authorization (wind-tide/API.md §14) or an AI Studio
// account by pasting its Cookie, which the aistudio-api pool or the BUILD
// pool (AIStudioToAPI) keeps (§15); the site keeps the ledger and never sees
// a token or stores a Cookie.

const (
	wtDonationFlowTTL      = 5 * time.Minute
	wtDonationCallbackMax  = 2048
	wtDonationTestInterval = 60 * time.Second
	wtDonationTestsPerDay  = 20
	wtDonationRemoveGrace  = 10 * time.Minute
	// wtAisDonateBodyMax leaves room for the JSON escaping of a Cookie of
	// service.WtAisCookieMaxBytes.
	wtAisDonateBodyMax = 3 * service.WtAisCookieMaxBytes
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
	errWtAisConsent           = common.NewMessage("Accept the risks before donating an account")
	errWtAisUsage             = common.NewMessage("Usage is not available for this pool")
	wtAisTestOK               = "The pool can still use this account"
	wtAisTestFailed           = "The pool can no longer sign in with this account"
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

// wtDonationReady answers the disabled message unless the site is wired to a
// CPA pool.
func wtDonationReady(c *gin.Context) bool {
	if service.WtDonationEnabled() {
		return true
	}
	common.ApiError(c, errWtDonationDisabled)
	return false
}

// wtDonationProviderReady answers the disabled message of the donation's
// pool unless that pool is configured.
func wtDonationProviderReady(c *gin.Context, donation *model.WtDonation) bool {
	switch donation.Provider {
	case model.WtAisProvider:
		return wtAisReady(c)
	case model.WtBuildProvider:
		return wtBuildReady(c)
	case model.WtWbProvider:
		return wtWbReady(c)
	}
	return wtDonationReady(c)
}

// wtProviderOf reads the provider query of the list, Antigravity unless AI
// Studio, Build or WorkBuddy is asked for, and checks that its pool is
// configured.
func wtProviderOf(c *gin.Context) (string, bool) {
	provider := c.DefaultQuery("provider", service.WtDonationProvider)
	switch provider {
	case service.WtDonationProvider:
		return provider, wtDonationReady(c)
	case model.WtAisProvider:
		return provider, wtAisReady(c)
	case model.WtBuildProvider:
		return provider, wtBuildReady(c)
	case model.WtWbProvider:
		return provider, wtWbReady(c)
	}
	common.ApiErrorT(c, "Invalid parameters")
	return "", false
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
		if donation.Provider != service.WtDonationProvider || (donation.Status != model.WtDonationActive && donation.Status != model.WtDonationFailed) {
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
		slots, err := model.CountWtDonationSlots(identity.UserID, service.WtDonationProvider)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if slots >= model.WtDonationMaxPerUser {
			common.ApiError(c, model.ErrWtDonationLimit)
			return
		}
		inPool, err := model.WtDonationEmailInPool(email, service.WtDonationProvider, 0)
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

// wtCookieDonationInput reads the {cookies, accepted} body of a Cookie
// donation and parses the Cookie into the header string the pools take and
// the fingerprint of the Google account. Every refusal is answered here; the
// Cookie text is never kept.
func wtCookieDonationInput(c *gin.Context) (header, fingerprint string, ok bool) {
	var input struct {
		Cookies  string `json:"cookies"`
		Accepted bool   `json:"accepted"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, wtAisDonateBodyMax)
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return "", "", false
	}
	if !input.Accepted {
		common.ApiErrorStatus(c, http.StatusBadRequest, errWtAisConsent)
		return "", "", false
	}
	if len(input.Cookies) > service.WtAisCookieMaxBytes {
		common.ApiErrorStatus(c, http.StatusBadRequest, model.ErrWtAisCookie)
		return "", "", false
	}
	header, fingerprint, ok = service.WtAisParseCookies(input.Cookies)
	if !ok {
		common.ApiErrorStatus(c, http.StatusBadRequest, model.ErrWtAisCookie)
		return "", "", false
	}
	return header, fingerprint, true
}

// wtCookieDonationSlot checks that the fingerprint is new to the provider's
// pool and that the member has a slot left; every refusal is answered here.
func wtCookieDonationSlot(c *gin.Context, userID int, provider, fingerprint string) bool {
	claimed, err := model.WtAisFingerprintInPool(provider, fingerprint)
	if err != nil {
		common.ApiError(c, err)
		return false
	}
	if claimed {
		common.ApiError(c, model.ErrWtDonationDuplicate)
		return false
	}
	slots, err := model.CountWtDonationSlots(userID, provider)
	if err != nil {
		common.ApiError(c, err)
		return false
	}
	if slots >= model.WtAisMaxPerUser {
		common.ApiError(c, model.ErrWtDonationLimit)
		return false
	}
	return true
}

// wtAcceptedDonation answers 202 with the new record's progress.
func wtAcceptedDonation(c *gin.Context, donation *model.WtDonation) {
	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"id": donation.Id, "status": model.WtDonationVerifying},
	})
}

// WtDonateAistudio hands a pasted AI Studio Cookie to the AIS pool and starts
// the verification that reads the account's email. The Cookie is parsed for
// its fingerprint, sent once and forgotten; the site keeps the pool's account
// id only.
func WtDonateAistudio(c *gin.Context) {
	if !wtAisReady(c) {
		return
	}
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		common.ApiError(c, errWtDonationSession)
		return
	}
	header, fingerprint, ok := wtCookieDonationInput(c)
	if !ok || !wtCookieDonationSlot(c, identity.UserID, model.WtAisProvider, fingerprint) {
		return
	}
	audit := map[string]any{"id": 0, "provider": model.WtAisProvider, "success": false}
	accountID, err := service.WtAisImportCookies(c.Request.Context(), header, identity.UserID)
	if err != nil {
		recordUserSecurityAudit(c, identity.UserID, "wt.donation_add", audit)
		if errors.Is(err, model.ErrWtAisCookie) {
			common.ApiErrorStatus(c, http.StatusBadRequest, err)
			return
		}
		common.ApiErrorStatus(c, http.StatusBadGateway, err)
		return
	}
	donation, err := model.CreateWtAisDonation(model.WtAisProvider, identity.UserID, accountID, fingerprint)
	if err != nil {
		if deleteErr := service.WtAisDeleteAccount(c.Request.Context(), accountID); deleteErr != nil {
			common.SysError(common.LogText("wt ais: an imported account of member %d could not be deleted from the pool after a database error", identity.UserID))
		}
		recordUserSecurityAudit(c, identity.UserID, "wt.donation_add", audit)
		common.ApiError(c, err)
		return
	}
	service.WtStartAisVerification(service.WtAisVerifyJob{
		DonationID:  donation.Id,
		UserID:      identity.UserID,
		AccountID:   accountID,
		DisplayName: model.WtDisplayNameOfUser(identity.UserID),
	})
	audit["id"], audit["success"] = donation.Id, true
	recordUserSecurityAudit(c, identity.UserID, "wt.donation_add", audit)
	wtAcceptedDonation(c, donation)
}

// WtDonateBuild turns a pasted AI Studio Cookie into an auth file of the
// BUILD pool, uploads it and starts the verification that watches the pool
// sign the account in. The Cookie is parsed for its fingerprint, sent once
// and forgotten; the site keeps the pool's account index only.
func WtDonateBuild(c *gin.Context) {
	if !wtBuildReady(c) {
		return
	}
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		common.ApiError(c, errWtDonationSession)
		return
	}
	header, fingerprint, ok := wtCookieDonationInput(c)
	if !ok || !wtCookieDonationSlot(c, identity.UserID, model.WtBuildProvider, fingerprint) {
		return
	}
	state, err := service.WtBuildStorageState(header, identity.UserID)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, model.ErrWtAisCookie)
		return
	}
	audit := map[string]any{"id": 0, "provider": model.WtBuildProvider, "success": false}
	index, err := service.WtBuildUploadAccount(c.Request.Context(), state)
	if err != nil {
		recordUserSecurityAudit(c, identity.UserID, "wt.donation_add", audit)
		status := http.StatusBadGateway
		if errors.Is(err, service.ErrWtBuildBusy) {
			status = http.StatusServiceUnavailable
		}
		common.ApiErrorStatus(c, status, err)
		return
	}
	donation, err := model.CreateWtAisDonation(model.WtBuildProvider, identity.UserID, strconv.Itoa(index), fingerprint)
	if err != nil {
		if deleteErr := service.WtBuildDeleteAccount(c.Request.Context(), index); deleteErr != nil {
			common.SysError(common.LogText("wt build: an uploaded account of member %d could not be deleted from the pool after a database error", identity.UserID))
		}
		recordUserSecurityAudit(c, identity.UserID, "wt.donation_add", audit)
		common.ApiError(c, err)
		return
	}
	service.WtStartBuildVerification(service.WtBuildVerifyJob{
		DonationID:  donation.Id,
		UserID:      identity.UserID,
		Index:       index,
		DisplayName: model.WtDisplayNameOfUser(identity.UserID),
	})
	audit["id"], audit["success"] = donation.Id, true
	recordUserSecurityAudit(c, identity.UserID, "wt.donation_add", audit)
	wtAcceptedDonation(c, donation)
}

// wtDonationItem is one row of the member's list (wind-tide/API.md §14 to
// §16); pool is the live state of the account, nil when there is none.
func wtDonationItem(donation *model.WtDonation, pool any) gin.H {
	item := gin.H{
		"id":          donation.Id,
		"provider":    donation.Provider,
		"email":       donation.Email,
		"nickname":    donation.Nickname,
		"status":      donation.Status,
		"fail_reason": donation.FailReason,
		"pool":        pool,
		"last_test":   nil,
		"created_at":  donation.CreatedAt,
		"verified_at": donation.VerifiedAt,
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

// wtDonationPoolOf is the live state of an active donation's account: from
// the credential list for Antigravity, where an unreachable pool shows as
// unknown rather than as a missing file, from the rotation table for AI
// Studio and from the pool status for Build and WorkBuddy, where an
// unreachable pool shows nothing.
func wtDonationPoolOf(donation *model.WtDonation, pool map[string]*service.WtCPACredential, rotation map[string]*service.WtAisRotationEntry, build *service.WtBuildStatus, wb *service.WtWbStatus, now time.Time) any {
	if donation.Status != model.WtDonationActive {
		return nil
	}
	switch donation.Provider {
	case model.WtAisProvider:
		if rotation == nil {
			return nil
		}
		return service.WtAisPoolStateOf(rotation[donation.CredentialName], now)
	case model.WtBuildProvider:
		index, err := strconv.Atoi(donation.CredentialName)
		if build == nil || err != nil {
			return nil
		}
		return service.WtBuildPoolStateOf(build, index)
	case model.WtWbProvider:
		if wb == nil {
			return nil
		}
		return service.WtWbPoolStateOf(wb, donation.CredentialName, now)
	}
	if pool == nil {
		return service.WtPoolInfo{State: service.WtPoolUnknown}
	}
	return service.WtPoolStateOf(pool[donation.CredentialName], now)
}

// WtListDonations is the member's donations to one pool, newest first, with
// the live state of the active ones from one cached pool listing.
func WtListDonations(c *gin.Context) {
	provider, ok := wtProviderOf(c)
	if !ok {
		return
	}
	donations, err := model.ListWtDonations(c.GetInt("id"), provider)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	anyActive := slices.ContainsFunc(donations, func(donation *model.WtDonation) bool {
		return donation.Status == model.WtDonationActive
	})
	var pool map[string]*service.WtCPACredential
	var rotation map[string]*service.WtAisRotationEntry
	var build *service.WtBuildStatus
	var wb *service.WtWbStatus
	maxSlots := model.WtDonationMaxPerUser
	switch {
	case provider == model.WtAisProvider:
		maxSlots = model.WtAisMaxPerUser
		if anyActive {
			rotation, _ = service.WtAisRotation(c.Request.Context())
		}
	case provider == model.WtBuildProvider:
		maxSlots = model.WtAisMaxPerUser
		if anyActive {
			build, _ = service.WtBuildStatusCached(c.Request.Context())
		}
	case provider == model.WtWbProvider:
		maxSlots = model.WtAisMaxPerUser
		if anyActive {
			wb, _ = service.WtWbStatusCached(c.Request.Context())
		}
	case anyActive:
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
		items = append(items, wtDonationItem(donation, wtDonationPoolOf(donation, pool, rotation, build, wb, now)))
	}
	common.ApiSuccess(c, gin.H{"max": maxSlots, "items": items})
}

// WtGetDonation is the progress of one donation; the dialog polls it while
// the verification runs. It reads the database only.
func WtGetDonation(c *gin.Context) {
	if !wtPoolReady(c) {
		return
	}
	donation, ok := wtDonationOf(c)
	if !ok {
		return
	}
	common.ApiSuccess(c, gin.H{
		"id":          donation.Id,
		"provider":    donation.Provider,
		"email":       donation.Email,
		"nickname":    donation.Nickname,
		"status":      donation.Status,
		"fail_reason": donation.FailReason,
	})
}

// WtGetDonationUsage is the remaining allowance of an active Antigravity
// donation, at most a minute old; the AI Studio pool has no usage figure.
func WtGetDonationUsage(c *gin.Context) {
	if !wtPoolReady(c) {
		return
	}
	donation, ok := wtDonationOf(c)
	if !ok {
		return
	}
	if donation.Provider != service.WtDonationProvider {
		common.ApiErrorStatus(c, http.StatusNotFound, errWtAisUsage)
		return
	}
	if !wtDonationReady(c) {
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

// WtTestDonation checks that the pool can still use the account, records the
// attempt with its latency, and holds each account to one test a minute and
// twenty a Beijing day. An Antigravity account is asked for its allowance
// afresh, an AI Studio account for its signed-in email; the pool failing is
// a recorded failure, not an error.
func WtTestDonation(c *gin.Context) {
	if !wtPoolReady(c) {
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
	if !wtDonationProviderReady(c, donation) {
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
	var usage *service.WtUsage
	passed, message := false, ""
	switch donation.Provider {
	case model.WtAisProvider:
		outcome, _, err := service.WtAisDetectEmail(c.Request.Context(), donation.CredentialName)
		passed = err == nil && outcome == service.WtAisDetectOK
		message = wtAisTestFailed
		if passed {
			message = wtAisTestOK
		}
	case model.WtBuildProvider:
		// The BUILD console has no probe of its own: the account passes while
		// the console still lists it as neither expired nor unreadable.
		index, _ := strconv.Atoi(donation.CredentialName)
		var account *service.WtBuildAccount
		if status, err := service.WtBuildStatusNow(c.Request.Context()); err == nil {
			account = status.Account(index)
		}
		passed = account != nil && !account.IsExpired && !account.IsInvalid
		message = wtAisTestFailed
		if passed {
			message = wtAisTestOK
		}
	case model.WtWbProvider:
		// The panel refreshes the token itself: the account passes while the
		// panel still lists it and has not given up on it.
		var account *service.WtWbAccount
		if status, err := service.WtWbStatusNow(c.Request.Context()); err == nil {
			account = status.Account(donation.CredentialName)
		}
		passed = account != nil && !account.Disabled
		message = wtWbTestFailed
		if passed {
			message = wtAisTestOK
		}
	default:
		var detail string
		var err error
		usage, detail, err = service.WtDonationUsage(c.Request.Context(), donation, true)
		passed = err == nil
		if err != nil {
			message = cmp.Or(detail, err.Error())
		}
	}
	latency := int(time.Since(began).Milliseconds())
	if recordErr := model.RecordWtDonationTest(donation.Id, passed, latency, message, day, testsToday+1); recordErr != nil {
		common.ApiError(c, recordErr)
		return
	}
	recordUserSecurityAudit(c, identity.UserID, "wt.donation_test", map[string]any{
		"id":         donation.Id,
		"provider":   donation.Provider,
		"success":    passed,
		"latency_ms": latency,
	})
	common.ApiSuccess(c, gin.H{"ok": passed, "latency_ms": latency, "message": message, "usage": usage})
}

// WtRemoveDonation takes the account out of its pool and closes the
// donation. One submitted less than ten minutes ago is left to the verifier,
// so the two do not race; the record stays when the pool could not delete
// the account. The pool page announces the removal of an active account.
func WtRemoveDonation(c *gin.Context) {
	if !wtPoolReady(c) {
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
	if !wtDonationProviderReady(c, donation) {
		return
	}
	audit := map[string]any{"id": donation.Id, "provider": donation.Provider, "success": false}
	if donation.CredentialName != "" {
		var err error
		switch donation.Provider {
		case model.WtAisProvider:
			err = service.WtAisDeleteAccount(c.Request.Context(), donation.CredentialName)
		case model.WtBuildProvider:
			if index, convErr := strconv.Atoi(donation.CredentialName); convErr == nil {
				err = service.WtBuildDeleteAccount(c.Request.Context(), index)
			}
		case model.WtWbProvider:
			err = service.WtWbRemoveAccount(c.Request.Context(), donation.CredentialName)
		default:
			err = service.WtCPADeleteCredential(c.Request.Context(), donation.CredentialName)
		}
		if err != nil {
			recordUserSecurityAudit(c, identity.UserID, "wt.donation_remove", audit)
			common.ApiError(c, err)
			return
		}
	}
	wasActive := donation.Status == model.WtDonationActive
	if err := model.RemoveWtDonation(donation.Id); err != nil {
		common.ApiError(c, err)
		return
	}
	kind := model.WtPoolEventAntigravityRemoved
	switch donation.Provider {
	case model.WtAisProvider:
		kind = model.WtPoolEventAisRemoved
		service.WtForgetAisRotation()
	case model.WtBuildProvider:
		kind = model.WtPoolEventBuildRemoved
		service.WtForgetBuildStatus()
	case model.WtWbProvider:
		kind = model.WtPoolEventWbRemoved
		service.WtForgetWbStatus()
	default:
		service.WtForgetCredentialUsage(donation.AuthIndex)
		service.WtForgetPoolCredentials()
	}
	if wasActive {
		if err := model.WtInsertPoolEvent(kind, donation.Provider, identity.UserID, model.WtDisplayNameOfUser(identity.UserID), 1, ""); err != nil {
			common.SysError(common.LogText("wt donation: the announcement of removed donation %d was not saved: %s", donation.Id, err.Error()))
		}
	}
	audit["success"] = true
	recordUserSecurityAudit(c, identity.UserID, "wt.donation_remove", audit)
	common.ApiSuccessT(c, "Donation removed", nil)
}
