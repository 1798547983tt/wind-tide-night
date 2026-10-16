package model

import (
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// wind-tide: check-in days follow Beijing time whatever the server time zone
// is, and the check-in reward is fixed (wind-tide/API.md §6).

var wtBeijing = time.FixedZone("UTC+8", 8*60*60)

// WtNow is the clock behind the Beijing check-in day and the signed avatar
// addresses. Tests replace it to pin the day.
var WtNow = time.Now

const wtCheckinDayLayout = "2006-01-02"

// WtCheckinDay is the Beijing (UTC+8) check-in day of t as YYYY-MM-DD.
func WtCheckinDay(t time.Time) string {
	return t.In(wtBeijing).Format(wtCheckinDayLayout)
}

// WtCheckinMonth is the Beijing (UTC+8) month of t as YYYY-MM.
func WtCheckinMonth(t time.Time) string {
	return t.In(wtBeijing).Format("2006-01")
}

// wtCheckinDayStart is the instant the Beijing check-in day of t begins.
func wtCheckinDayStart(t time.Time) time.Time {
	year, month, day := t.In(wtBeijing).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, wtBeijing)
}

// WtCheckinReward is the fixed check-in reward in quota. The upstream range
// settings are kept; the larger bound is paid, so both default to the same
// value. A misconfigured negative bound never debits the wallet.
func WtCheckinReward() int {
	setting := operation_setting.GetCheckinSetting()
	return max(setting.MinQuota, setting.MaxQuota, 0)
}

// WtCheckinStreak counts the consecutive check-in days up to the latest
// check-in day: it ends today once the user has checked in today, and ends
// yesterday while today is still open. A missed day resets it to zero.
func WtCheckinStreak(userId int, now time.Time) (int, error) {
	today := wtCheckinDayStart(now)
	var dates []string
	if err := DB.Model(&Checkin{}).
		Where("user_id = ? AND checkin_date <= ?", userId, today.Format(wtCheckinDayLayout)).
		Order("checkin_date DESC").
		Pluck("checkin_date", &dates).Error; err != nil {
		return 0, err
	}
	expected := today
	if len(dates) > 0 && dates[0] != today.Format(wtCheckinDayLayout) {
		expected = today.AddDate(0, 0, -1)
	}
	streak := 0
	for _, date := range dates {
		if date != expected.Format(wtCheckinDayLayout) {
			break
		}
		streak++
		expected = expected.AddDate(0, 0, -1)
	}
	return streak, nil
}
