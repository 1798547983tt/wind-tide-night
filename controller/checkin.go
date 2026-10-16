package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// GetCheckinStatus 获取用户签到状态和历史记录
func GetCheckinStatus(c *gin.Context) {
	setting := operation_setting.GetCheckinSetting()
	if !setting.Enabled {
		common.ApiErrorT(c, "Check-in feature is not enabled")
		return
	}
	userId := c.GetInt("id")
	// 获取月份参数，默认为当前月份
	// wind-tide: months, days and the streak follow Beijing time, and the
	// reward is fixed (wind-tide/API.md §6).
	now := model.WtNow()
	month := c.DefaultQuery("month", model.WtCheckinMonth(now))

	stats, err := model.GetUserCheckinStats(userId, month)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	streak, err := model.WtCheckinStreak(userId, now)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":          setting.Enabled,
			"min_quota":        setting.MinQuota,
			"max_quota":        setting.MaxQuota,
			"stats":            stats,
			"today":            model.WtCheckinDay(now),
			"checked_in_today": stats["checked_in_today"],
			"streak":           streak,
			"reward_quota":     model.WtCheckinReward(),
		},
	})
}

// DoCheckin 执行用户签到
func DoCheckin(c *gin.Context) {
	setting := operation_setting.GetCheckinSetting()
	if !setting.Enabled {
		common.ApiErrorT(c, "Check-in feature is not enabled")
		return
	}

	userId := c.GetInt("id")

	checkin, err := model.UserCheckin(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(userId, model.LogTypeSystem, common.NewMessage("Daily check-in, received {{quota}}", map[string]any{"quota": logger.FormatQuota(checkin.QuotaAwarded)}))
	// wind-tide: the streak including this check-in. The check-in is already
	// committed, so a failed count is logged and reported as 0.
	streak, err := model.WtCheckinStreak(userId, model.WtNow())
	if err != nil {
		common.SysError(common.LogText("failed to count the check-in streak of user %d: %s", userId, err.Error()))
	}
	common.ApiSuccessT(c, "Check-in successful", gin.H{
		"quota_awarded": checkin.QuotaAwarded,
		"checkin_date":  checkin.CheckinDate,
		"streak":        streak,
	})
}
