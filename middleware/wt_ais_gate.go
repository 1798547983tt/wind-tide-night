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

// wind-tide: the gate of the AIS playground pool (wind-tide/API.md §15). A
// call of an "AIS PG-" model costs one of the member's daily calls of that
// model: donors of AI Studio accounts have the model's share per active
// account plus what they took from the public pool, everybody else has none.
// A call the upstream failed is given back and counted as failed. The owner
// tests freely and uncounted.

// errWtAisGateUnavailable is the answer when the ledger cannot be written.
var errWtAisGateUnavailable = common.NewMessage("The AIS playground pool is temporarily unavailable")

// WtAisGate runs right after Distribute, so the original model name is in
// the context. Models outside the pool pass without a database query.
func WtAisGate() gin.HandlerFunc {
	return func(c *gin.Context) {
		modelName := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
		if !strings.HasPrefix(modelName, service.WtAisModelPrefix) {
			c.Next()
			return
		}
		spec, known := model.WtAisModelByName(modelName)
		if !known {
			abortWithOpenAiMessage(c, http.StatusBadRequest, model.ErrWtAisUnknownModel.Error())
			return
		}
		if !service.WtAisEnabled() {
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, service.ErrWtAisNotConfigured.Error())
			return
		}
		if common.GetContextKeyInt(c, constant.ContextKeyUserRole) == common.RoleRootUser || c.GetInt("role") == common.RoleRootUser {
			c.Next()
			return
		}
		userID := c.GetInt("id")
		day := model.WtCheckinDay(model.WtNow())
		remaining, err := model.WtAisReserveCall(userID, day, spec.Upstream)
		switch {
		case errors.Is(err, model.ErrWtAisNotDonor):
			abortWithOpenAiMessage(c, http.StatusForbidden, err.Error())
			return
		case errors.Is(err, model.ErrWtAisExhausted):
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, err.Error())
			return
		case err != nil:
			common.SysError(common.LogText("wt ais: the %s call of member %d could not be reserved: %s", spec.Upstream, userID, err.Error()))
			abortWithOpenAiMessage(c, http.StatusInternalServerError, errWtAisGateUnavailable.Error())
			return
		}
		c.Header("X-WT-AIS-Remaining", strconv.Itoa(remaining))
		c.Next()
		if modelRequestSucceeded(c) {
			return
		}
		if err := model.WtAisRefundCall(userID, day, spec.Upstream); err != nil {
			common.SysError(common.LogText("wt ais: the failed %s call of member %d could not be refunded: %s", spec.Upstream, userID, err.Error()))
		}
	}
}
