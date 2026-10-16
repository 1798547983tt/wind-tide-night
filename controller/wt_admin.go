package controller

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// wind-tide: the owner's allowlist and entry request management
// (wind-tide/API.md §5). The routes are root-only.

// wtPageQuery reads p and page_size like the upstream lists, and also turns a
// non-positive page or page size into the default instead of an unbounded
// query.
func wtPageQuery(c *gin.Context) *common.PageInfo {
	pageInfo := common.GetPageQuery(c)
	pageInfo.Page = max(pageInfo.Page, 1)
	if pageInfo.PageSize < 1 {
		pageInfo.PageSize = common.ItemsPerPage
	}
	return pageInfo
}

func WtListAllowlist(c *gin.Context) {
	status := c.Query("status")
	if status != "" && status != model.WtAllowlistActive && status != model.WtAllowlistRemoved {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	pageInfo := wtPageQuery(c)
	items, total, err := model.WtListAllowlist(c.Query("keyword"), status, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func WtAddAllowlistEntry(c *gin.Context) {
	var input model.WtAllowlistInput
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	item, err := model.WtAddAllowlistEntry(input, time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, item.UserId, "wt.allowlist_add", wtAllowlistAuditParams(item))
	common.ApiSuccess(c, item)
}

func WtUpdateAllowlistNote(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	var request struct {
		Note *string `json:"note"`
	}
	if err != nil || common.DecodeJson(c.Request.Body, &request) != nil || request.Note == nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	item, err := model.WtUpdateAllowlistNote(id, *request.Note, time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, item.UserId, "wt.allowlist_update", wtAllowlistAuditParams(item))
	common.ApiSuccess(c, item)
}

func WtRemoveAllowlistEntry(c *gin.Context) {
	changeWtAllowlistStatus(c, model.WtAllowlistRemoved)
}

func WtRestoreAllowlistEntry(c *gin.Context) {
	changeWtAllowlistStatus(c, model.WtAllowlistActive)
}

// changeWtAllowlistStatus removes or restores an allowlist entry together with
// its account and audits the change. Disabling or enabling an account takes
// the owner's step-up verification that upstream user management asks for:
// scope admin.user.manage with the account and the action disable or enable.
// A removal also records how many access tokens and API keys it revoked.
func changeWtAllowlistStatus(c *gin.Context, status string) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	accountID, err := model.WtAllowlistStatusAccount(id, status)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	action, accountAction := "wt.allowlist_remove", "disable"
	if status == model.WtAllowlistActive {
		action, accountAction = "wt.allowlist_restore", "enable"
	}
	verificationMethod := ""
	if accountID != 0 {
		authorization := requireAdminUserProof(c, service.VerificationScopeAdminUserManage, service.AdminUserManageContext{UserID: accountID, Action: accountAction})
		if authorization == nil {
			return
		}
		verificationMethod = authorization.Method
	}
	var item *model.WtAllowlistItem
	var revoked model.WtRevokedCredentials
	if status == model.WtAllowlistRemoved {
		item, revoked, err = model.WtRemoveAllowlistEntry(id, accountID, time.Now())
	} else {
		item, err = model.WtRestoreAllowlistEntry(id, accountID, time.Now())
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	params := wtAllowlistAuditParams(item)
	if verificationMethod != "" {
		params["verification_method"] = verificationMethod
	}
	if status == model.WtAllowlistRemoved {
		params["access_tokens"], params["relay_keys"] = revoked.AccessTokens, revoked.RelayKeys
	}
	recordManageAuditFor(c, item.UserId, action, params)
	common.ApiSuccess(c, item)
}

// wtAllowlistAuditParams identifies an allowlist entry in audit logs; the note
// is left out.
func wtAllowlistAuditParams(item *model.WtAllowlistItem) map[string]any {
	return map[string]any{
		"id":               item.Id,
		"discord_id":       item.DiscordId,
		"discord_username": item.DiscordUsername,
		"status":           item.Status,
	}
}

func WtListEntryRequests(c *gin.Context) {
	status := c.Query("status")
	if status != "" && status != model.WtEntryPending && status != model.WtEntryApproved && status != model.WtEntryRejected {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	pageInfo := wtPageQuery(c)
	items, total, err := model.WtListEntryRequests(status, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func WtCountPendingEntryRequests(c *gin.Context) {
	count, err := model.WtCountPendingEntryRequests()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"count": count})
}

func WtApproveEntryRequest(c *gin.Context) {
	decideWtEntryRequest(c, model.WtApproveEntryRequest, "wt.entry_approve")
}

func WtRejectEntryRequest(c *gin.Context) {
	decideWtEntryRequest(c, model.WtRejectEntryRequest, "wt.entry_reject")
}

// decideWtEntryRequest approves or rejects an entry request and audits the
// decision.
func decideWtEntryRequest(c *gin.Context, decide func(int, time.Time) (*model.WtEntryRequestItem, error), action string) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	item, err := decide(id, time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, 0, action, map[string]any{
		"id":               item.Id,
		"discord_id":       item.DiscordId,
		"discord_username": item.DiscordUsername,
	})
	common.ApiSuccess(c, item)
}
