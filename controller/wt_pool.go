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
// Antigravity pool, the member's AIS PG calls per model and the public pools
// they can give to and take from, the announcement feed and the leaderboards.

const (
	wtPoolEventsPage      = 50
	wtAisLeaderboardLimit = 20
)

// wtPoolReady answers the disabled message unless the site is wired to at
// least one pool.
func wtPoolReady(c *gin.Context) bool {
	if service.WtDonationEnabled() || service.WtAisEnabled() {
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

// WtGetPool is the pool page in one read: each tank is null while its pool
// is not configured.
func WtGetPool(c *gin.Context) {
	if !wtPoolReady(c) {
		return
	}
	reply := gin.H{"antigravity": nil, "aistudio": nil}
	if service.WtDonationEnabled() {
		reply["antigravity"] = service.WtPoolLevelSnapshot()
	}
	if service.WtAisEnabled() {
		balance, err := model.WtAisBalanceOfDay(c.GetInt("id"), model.WtCheckinDay(model.WtNow()))
		if err != nil {
			common.ApiError(c, err)
			return
		}
		reply["aistudio"] = balance
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
// be one of the pool's upstream models, the count a whole number of 1 or
// more.
func wtAisMoveOf(c *gin.Context) (*model.WtAisModelSpec, int, bool) {
	var input struct {
		Model string `json:"model"`
		Count *int   `json:"count"`
	}
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiError(c, model.ErrWtAisCount)
		return nil, 0, false
	}
	spec, known := model.WtAisModelByUpstream(strings.TrimSpace(input.Model))
	if !known {
		common.ApiErrorStatus(c, http.StatusBadRequest, model.ErrWtAisUnknownModel)
		return nil, 0, false
	}
	if input.Count == nil || *input.Count < 1 {
		common.ApiError(c, model.ErrWtAisCount)
		return nil, 0, false
	}
	return spec, *input.Count, true
}

// WtGiveAisCalls moves some of the member's remaining calls of one model into
// that model's public pool.
func WtGiveAisCalls(c *gin.Context) {
	if !wtAisReady(c) {
		return
	}
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
	remaining, poolBalance, err := model.WtAisGive(identity.UserID, day, spec.Upstream, count, model.WtDisplayNameOfUser(identity.UserID))
	recordUserSecurityAudit(c, identity.UserID, "wt.ais_give", map[string]any{"model": spec.Upstream, "count": count, "success": err == nil})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"model": spec.Upstream, "remaining": remaining, "pool_balance": poolBalance})
}

// WtTakeAisCalls moves calls from one model's public pool to the member's day.
func WtTakeAisCalls(c *gin.Context) {
	if !wtAisReady(c) {
		return
	}
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
	remaining, poolBalance, takeLeft, err := model.WtAisTake(identity.UserID, day, spec.Upstream, count, model.WtDisplayNameOfUser(identity.UserID))
	recordUserSecurityAudit(c, identity.UserID, "wt.ais_take", map[string]any{"model": spec.Upstream, "count": count, "success": err == nil})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"model": spec.Upstream, "remaining": remaining, "pool_balance": poolBalance, "take_left": takeLeft})
}

// WtGetAisLeaderboard is, for every model, the top twenty by calls and by
// failed calls, today and of all time.
func WtGetAisLeaderboard(c *gin.Context) {
	if !wtAisReady(c) {
		return
	}
	boards, err := model.WtAisLeaderboards(model.WtCheckinDay(model.WtNow()), wtAisLeaderboardLimit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"models": boards})
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
