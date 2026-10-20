package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// wind-tide: the gate of the donated-account pools (wind-tide/API.md §15) and
// the counter behind the member leaderboards (§7). A call of an "AIS PG-" or
// "Build-" model costs one of the member's daily calls of that model: donors
// of the pool's accounts have the model's share per active account plus what
// they took from the public pool, everybody else has none. A call the
// upstream failed is given back and counted as failed. The owner tests freely
// and uncounted. Every other relay call passes untouched; its outcome moves
// the member's day counters and streaks.

// errWtPoolGateUnavailable is the answer when the ledger cannot be written.
var errWtPoolGateUnavailable = common.NewMessage("The pool is temporarily unavailable")

// WtPoolGate runs right after Distribute, so the original model name is in
// the context. Models outside the pools pass without a database query.
func WtPoolGate() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt("id")
		modelName := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
		provider := wtPoolProviderOf(modelName)
		if provider == "" {
			c.Next()
			model.WtCountCall(userID, modelRequestSucceeded(c), model.WtNow())
			return
		}
		spec, known := model.WtAisModelByName(modelName)
		if !known || spec.Provider != provider {
			abortWithOpenAiMessage(c, http.StatusBadRequest, wtPoolUnknownModel(provider).Error())
			return
		}
		if !wtPoolEnabled(provider) {
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, wtPoolNotConfigured(provider).Error())
			return
		}
		if common.GetContextKeyInt(c, constant.ContextKeyUserRole) == common.RoleRootUser || c.GetInt("role") == common.RoleRootUser {
			c.Next()
			return
		}
		day := model.WtCheckinDay(model.WtNow())
		remaining, err := model.WtAisReserveCall(userID, day, spec.Key)
		switch {
		case errors.Is(err, model.ErrWtAisNotDonor), errors.Is(err, model.ErrWtBuildNotDonor):
			abortWithOpenAiMessage(c, http.StatusForbidden, err.Error())
			return
		case errors.Is(err, model.ErrWtAisExhausted), errors.Is(err, model.ErrWtBuildExhausted):
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, err.Error())
			return
		case err != nil:
			common.SysError(common.LogText("wt pool: the %s call of member %d could not be reserved: %s", spec.Key, userID, err.Error()))
			abortWithOpenAiMessage(c, http.StatusInternalServerError, errWtPoolGateUnavailable.Error())
			return
		}
		c.Header("X-WT-AIS-Remaining", strconv.Itoa(remaining))
		c.Next()
		ok := modelRequestSucceeded(c)
		model.WtCountCall(userID, ok, model.WtNow())
		if ok {
			return
		}
		if err := model.WtAisRefundCall(userID, day, spec.Key); err != nil {
			common.SysError(common.LogText("wt pool: the failed %s call of member %d could not be refunded: %s", spec.Key, userID, err.Error()))
		}
	}
}

// wtPoolProviderOf tells a pool model by its prefix; "" for any other model.
func wtPoolProviderOf(modelName string) string {
	switch {
	case strings.HasPrefix(modelName, service.WtAisModelPrefix):
		return model.WtAisProvider
	case strings.HasPrefix(modelName, service.WtBuildModelPrefix):
		return model.WtBuildProvider
	}
	return ""
}

func wtPoolEnabled(provider string) bool {
	if provider == model.WtBuildProvider {
		return service.WtBuildEnabled()
	}
	return service.WtAisEnabled()
}

func wtPoolUnknownModel(provider string) *common.Message {
	if provider == model.WtBuildProvider {
		return model.ErrWtBuildUnknownModel
	}
	return model.ErrWtAisUnknownModel
}

func wtPoolNotConfigured(provider string) *common.Message {
	if provider == model.WtBuildProvider {
		return service.ErrWtBuildNotConfigured
	}
	return service.ErrWtAisNotConfigured
}
