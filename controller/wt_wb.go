package controller

import (
	"cmp"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// wind-tide: donated WorkBuddy accounts (wind-tide/API.md §16). A member
// accepts the risks, the site asks the workbuddy2api panel for Tencent's
// sign-in link, the member signs in there and the panel collects the
// account's token; the site follows the sign-in (service/wt_wb_verify.go)
// and keeps the account id and the nickname only. The pool has no call
// ledger: every member may use the WorkBuddy channel.

const wtWbDonateBodyMax = 4 << 10

var (
	errWtWbRealm   = common.NewMessage("Choose the CodeBuddy or the WorkBuddy sign-in")
	wtWbTestFailed = "The pool can no longer use this account"
)

// wtWbReady answers the not-configured message unless the site is wired to
// a WorkBuddy pool.
func wtWbReady(c *gin.Context) bool {
	if service.WtWbEnabled() {
		return true
	}
	common.ApiError(c, service.ErrWtWbNotConfigured)
	return false
}

// WtDonateWorkbuddy begins a WorkBuddy donation: it checks the consent and
// the member's slots, asks the pool for Tencent's sign-in link of the chosen
// realm, records the donation as verifying and starts following the
// sign-in. The answer carries the link for the member to open; the pool's
// session behind it stays in the record.
func WtDonateWorkbuddy(c *gin.Context) {
	if !wtWbReady(c) {
		return
	}
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		common.ApiError(c, errWtDonationSession)
		return
	}
	var input struct {
		Realm    string `json:"realm"`
		Accepted bool   `json:"accepted"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, wtWbDonateBodyMax)
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	if !input.Accepted {
		common.ApiErrorStatus(c, http.StatusBadRequest, errWtAisConsent)
		return
	}
	realm := cmp.Or(strings.TrimSpace(input.Realm), service.WtWbRealmCN)
	if !service.WtWbValidRealm(realm) {
		common.ApiErrorStatus(c, http.StatusBadRequest, errWtWbRealm)
		return
	}
	slots, err := model.CountWtDonationSlots(identity.UserID, model.WtWbProvider)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if slots >= model.WtAisMaxPerUser {
		common.ApiError(c, model.ErrWtDonationLimit)
		return
	}
	audit := map[string]any{"id": 0, "provider": model.WtWbProvider, "realm": realm, "success": false}
	signIn, err := service.WtWbStartSignIn(c.Request.Context(), realm)
	if err != nil {
		recordUserSecurityAudit(c, identity.UserID, "wt.donation_add", audit)
		common.ApiErrorStatus(c, http.StatusBadGateway, err)
		return
	}
	donation, err := model.CreateWtWbDonation(identity.UserID, signIn.State)
	if err != nil {
		recordUserSecurityAudit(c, identity.UserID, "wt.donation_add", audit)
		common.ApiError(c, err)
		return
	}
	service.WtStartWbVerification(service.WtWbVerifyJob{
		DonationID:  donation.Id,
		UserID:      identity.UserID,
		State:       signIn.State,
		DisplayName: model.WtDisplayNameOfUser(identity.UserID),
	})
	audit["id"], audit["success"] = donation.Id, true
	recordUserSecurityAudit(c, identity.UserID, "wt.donation_add", audit)
	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"id":                donation.Id,
			"status":            model.WtDonationVerifying,
			"authorization_url": signIn.URL,
			"expires_at":        model.WtNow().Add(service.WtWbSignInTTL).Unix(),
		},
	})
}
