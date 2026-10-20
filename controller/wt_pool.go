package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// wind-tide: the pool page (wind-tide/API.md §15): the level of the
// Antigravity pool, the member's calls per model in the AIS PG and BUILD
// pools with the public pools they can give to and take from, and the
// announcement feed.

const wtPoolEventsPage = 50

// wtPoolReady answers the disabled message unless the site is wired to at
// least one pool.
func wtPoolReady(c *gin.Context) bool {
	if service.WtDonationEnabled() || service.WtAisEnabled() || service.WtBuildEnabled() || service.WtWbEnabled() {
		return true
	}
	common.ApiError(c, errWtDonationDisabled)
	return false
}

// wtAisReady answers the not-configured message unless the site is wired to
// an AI Studio pool.
func wtAisReady(c *gin.Context) bool {
	if service.WtAisEnabled() {
		return true
	}
	common.ApiError(c, service.ErrWtAisNotConfigured)
	return false
}

// wtBuildReady answers the not-configured message unless the site is wired
// to a BUILD pool.
func wtBuildReady(c *gin.Context) bool {
	if service.WtBuildEnabled() {
		return true
	}
	common.ApiError(c, service.ErrWtBuildNotConfigured)
	return false
}

// wtSpecReady answers the not-configured message of the model's pool unless
// that pool is wired.
func wtSpecReady(c *gin.Context, spec *model.WtAisModelSpec) bool {
	if spec.Provider == model.WtBuildProvider {
		return wtBuildReady(c)
	}
	return wtAisReady(c)
}

// WtGetPool is the pool page in one read: each tank is null while its pool
// is not configured. The WorkBuddy tank reads the panel's cached status; a
// panel that does not answer shows a tank without a reading.
func WtGetPool(c *gin.Context) {
	if !wtPoolReady(c) {
		return
	}
	reply := gin.H{"antigravity": nil, "aistudio": nil, "build": nil, "workbuddy": nil}
	if service.WtDonationEnabled() {
		reply["antigravity"] = service.WtPoolLevelSnapshot()
	}
	if service.WtWbEnabled() {
		level := service.WtWbPoolLevel{}
		if status, err := service.WtWbStatusCached(c.Request.Context()); err == nil {
			level = service.WtWbPoolLevelOf(status)
		}
		reply["workbuddy"] = level
	}
	day := model.WtCheckinDay(model.WtNow())
	for _, pool := range []struct {
		key      string
		provider string
		enabled  bool
	}{
		{"aistudio", model.WtAisProvider, service.WtAisEnabled()},
		{"build", model.WtBuildProvider, service.WtBuildEnabled()},
	} {
		if !pool.enabled {
			continue
		}
		balance, err := model.WtAisBalanceOfDay(c.GetInt("id"), day, pool.provider)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		reply[pool.key] = balance
	}
	events, err := model.WtListPoolEvents(0, wtPoolEventsPage)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	reply["events"] = events
	common.ApiSuccess(c, reply)
}

// wtAisMoveOf reads the {model, count} body of a give or take: the model must
// be the ledger key of a pool model whose pool is configured, the count a
// whole number of 1 or more.
func wtAisMoveOf(c *gin.Context) (*model.WtAisModelSpec, int, bool) {
	var input struct {
		Model string `json:"model"`
		Count *int   `json:"count"`
	}
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiError(c, model.ErrWtAisCount)
		return nil, 0, false
	}
	spec, known := model.WtAisModelByKey(strings.TrimSpace(input.Model))
	if !known {
		common.ApiErrorStatus(c, http.StatusBadRequest, model.ErrWtAisUnknownModel)
		return nil, 0, false
	}
	if !wtSpecReady(c, spec) {
		return nil, 0, false
	}
	if input.Count == nil || *input.Count < 1 {
		common.ApiError(c, model.ErrWtAisCount)
		return nil, 0, false
	}
	return spec, *input.Count, true
}

// WtGiveAisCalls moves some of the member's remaining calls of one model into
// that model's public pool. The model's own pool answers when it is not
// configured (wtAisMoveOf), so no site-wide check comes first.
func WtGiveAisCalls(c *gin.Context) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		common.ApiError(c, errWtDonationSession)
		return
	}
	spec, count, ok := wtAisMoveOf(c)
	if !ok {
		return
	}
	day := model.WtCheckinDay(model.WtNow())
	remaining, poolBalance, err := model.WtAisGive(identity.UserID, day, spec.Key, count, model.WtDisplayNameOfUser(identity.UserID))
	recordUserSecurityAudit(c, identity.UserID, "wt.ais_give", map[string]any{"model": spec.Key, "count": count, "success": err == nil})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"model": spec.Key, "remaining": remaining, "pool_balance": poolBalance})
}

// WtTakeAisCalls moves calls from one model's public pool to the member's day.
func WtTakeAisCalls(c *gin.Context) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		common.ApiError(c, errWtDonationSession)
		return
	}
	spec, count, ok := wtAisMoveOf(c)
	if !ok {
		return
	}
	day := model.WtCheckinDay(model.WtNow())
	remaining, poolBalance, takeLeft, err := model.WtAisTake(identity.UserID, day, spec.Key, count, model.WtDisplayNameOfUser(identity.UserID))
	recordUserSecurityAudit(c, identity.UserID, "wt.ais_take", map[string]any{"model": spec.Key, "count": count, "success": err == nil})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"model": spec.Key, "remaining": remaining, "pool_balance": poolBalance, "take_left": takeLeft})
}

// WtListPoolEvents is the announcement feed, newest first; after narrows it
// to the entries newer than an id the page already has.
func WtListPoolEvents(c *gin.Context) {
	if !wtPoolReady(c) {
		return
	}
	after, err := strconv.Atoi(c.DefaultQuery("after", "0"))
	if err != nil || after < 0 {
		after = 0
	}
	events, err := model.WtListPoolEvents(after, wtPoolEventsPage)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": events})
}
