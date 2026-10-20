package model

import (
	"cmp"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// wind-tide: member leaderboards and the public home page numbers
// (wind-tide/API.md §7 and §9).

const (
	WtBoardCalls         = "calls"
	WtBoardFailed        = "failed"
	WtBoardSuccessStreak = "success_streak"
	WtBoardFailStreak    = "fail_streak"
	WtBoardBalance       = "balance"

	WtPeriodToday = "today"
	WtPeriodWeek  = "week"
	WtPeriodAll   = "all"

	// WtLeaderboardTopRanks is how many ranks a leaderboard shows.
	WtLeaderboardTopRanks = 50
)

// WtKnownBoard reports whether board names one of the member boards.
func WtKnownBoard(board string) bool {
	switch board {
	case WtBoardCalls, WtBoardFailed, WtBoardSuccessStreak, WtBoardFailStreak, WtBoardBalance:
		return true
	}
	return false
}

// WtBoardHasPeriod reports whether the board counts over today, the week or
// all time; the streak boards and the balance board stand for the moment.
func WtBoardHasPeriod(board string) bool {
	return board == WtBoardCalls || board == WtBoardFailed
}

// WtLeaderboardItem is one ranked member. Current is set on the streak
// boards: the run the member is on now, beside the best run that ranks them.
type WtLeaderboardItem struct {
	Rank        int    `json:"rank"`
	UserId      int    `json:"user_id"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Value       int64  `json:"value"`
	Current     int64  `json:"current,omitempty"`
}

// WtLeaderboardRank is the requester's own place on a leaderboard.
type WtLeaderboardRank struct {
	Rank  int   `json:"rank"`
	Value int64 `json:"value"`
}

// WtLeaderboard ranks every eligible member of one board and period. Members
// with equal values share a rank and the next rank skips (1, 2, 2, 4).
type WtLeaderboard struct {
	Board     string
	Period    string
	Ranked    []WtLeaderboardItem
	UpdatedAt int64
}

// Top returns the members within the first WtLeaderboardTopRanks ranks that
// have a value; members without a call, a streak or a balance are not listed.
func (board *WtLeaderboard) Top() []WtLeaderboardItem {
	items := make([]WtLeaderboardItem, 0, min(len(board.Ranked), WtLeaderboardTopRanks))
	for _, item := range board.Ranked {
		if item.Value <= 0 || item.Rank > WtLeaderboardTopRanks {
			break
		}
		items = append(items, item)
	}
	return items
}

// Me returns the place of userId, or nil when the user is not ranked: the
// owner, administrators and disabled accounts never are.
func (board *WtLeaderboard) Me(userId int) *WtLeaderboardRank {
	for _, item := range board.Ranked {
		if item.UserId == userId {
			return &WtLeaderboardRank{Rank: item.Rank, Value: item.Value}
		}
	}
	return nil
}

// wtCallWindow is the [start, end) range of created_at for a call period:
// the Beijing check-in day, or the seven check-in days ending today.
func wtCallWindow(period string, now time.Time) (int64, int64) {
	today := wtCheckinDayStart(now)
	end := today.AddDate(0, 0, 1).Unix()
	if period == WtPeriodWeek {
		return today.AddDate(0, 0, -6).Unix(), end
	}
	return today.Unix(), end
}

// wtCheckinDays lists the check-in days of a period: today, or the seven
// days ending today.
func wtCheckinDays(period string, now time.Time) []string {
	today := wtCheckinDayStart(now)
	count := 1
	if period == WtPeriodWeek {
		count = 7
	}
	days := make([]string, 0, count)
	for i := count - 1; i >= 0; i-- {
		days = append(days, WtCheckinDay(today.AddDate(0, 0, -i)))
	}
	return days
}

// BuildWtLeaderboard ranks the enabled members below the administrator role.
// The calls board counts successful consume logs (type 2) in the log database
// for today and week, and uses users.request_count for all; the failed board
// sums the day counters of wt_call_days; the streak boards rank the best run
// of wt_call_streaks and carry the current one; the balance board uses
// users.quota.
func BuildWtLeaderboard(board, period string, now time.Time) (*WtLeaderboard, error) {
	var members []User
	if err := DB.Select("id", "username", "display_name", "discord_id", "quota", "request_count").
		Where("role < ? AND status = ?", common.RoleAdminUser, common.UserStatusEnabled).
		Find(&members).Error; err != nil {
		return nil, err
	}
	calls := map[int]int64{}
	if board == WtBoardCalls && period != WtPeriodAll {
		start, end := wtCallWindow(period, now)
		var counts []struct {
			UserId int
			Calls  int64
		}
		if err := LOG_DB.Model(&Log{}).Select("user_id, COUNT(*) AS calls").
			Where("type = ? AND created_at >= ? AND created_at < ?", LogTypeConsume, start, end).
			Group("user_id").Scan(&counts).Error; err != nil {
			return nil, err
		}
		for _, count := range counts {
			calls[count.UserId] = count.Calls
		}
	}
	failed := map[int]int64{}
	if board == WtBoardFailed {
		var counts []struct {
			UserId int
			Failed int64
		}
		query := DB.Model(&WtCallDay{}).Select("user_id, COALESCE(SUM(failed), 0) AS failed")
		if period != WtPeriodAll {
			query = query.Where("day IN ?", wtCheckinDays(period, now))
		}
		if err := query.Group("user_id").Scan(&counts).Error; err != nil {
			return nil, err
		}
		for _, count := range counts {
			failed[count.UserId] = count.Failed
		}
	}
	streaks := map[int]WtCallStreak{}
	if board == WtBoardSuccessStreak || board == WtBoardFailStreak {
		var rows []WtCallStreak
		if err := DB.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			streaks[row.UserId] = row
		}
	}
	discordIDs := make([]string, 0, len(members))
	for _, member := range members {
		if member.DiscordId != "" {
			discordIDs = append(discordIDs, member.DiscordId)
		}
	}
	profiles, err := wtProfiles(discordIDs)
	if err != nil {
		return nil, err
	}

	ranked := make([]WtLeaderboardItem, 0, len(members))
	for _, member := range members {
		item := WtLeaderboardItem{
			UserId:      member.Id,
			DisplayName: member.DisplayName,
			AvatarURL:   WtAvatarURL(member.DiscordId, profiles[member.DiscordId].AvatarHash),
		}
		if item.DisplayName == "" {
			item.DisplayName = member.Username
		}
		switch {
		case board == WtBoardBalance:
			item.Value = int64(member.Quota)
		case board == WtBoardFailed:
			item.Value = failed[member.Id]
		case board == WtBoardSuccessStreak:
			item.Value, item.Current = streaks[member.Id].BestSuccess, streaks[member.Id].SuccessRun
		case board == WtBoardFailStreak:
			item.Value, item.Current = streaks[member.Id].BestFail, streaks[member.Id].FailRun
		case period == WtPeriodAll:
			item.Value = int64(member.RequestCount)
		default:
			item.Value = calls[member.Id]
		}
		ranked = append(ranked, item)
	}
	slices.SortFunc(ranked, func(a, b WtLeaderboardItem) int {
		if a.Value != b.Value {
			return cmp.Compare(b.Value, a.Value)
		}
		return cmp.Compare(a.UserId, b.UserId)
	})
	for i := range ranked {
		ranked[i].Rank = i + 1
		if i > 0 && ranked[i].Value == ranked[i-1].Value {
			ranked[i].Rank = ranked[i-1].Rank
		}
	}
	return &WtLeaderboard{Board: board, Period: period, Ranked: ranked, UpdatedAt: now.Unix()}, nil
}

// WtPublicStats are the site totals shown to visitors on the home page.
type WtPublicStats struct {
	Members            int64 `json:"members"`
	CallsToday         int64 `json:"calls_today"`
	RewardedQuotaTotal int64 `json:"rewarded_quota_total"`
	UpdatedAt          int64 `json:"-"`
}

// GetWtPublicStats counts the enabled members, today's successful calls of the
// whole site (Beijing day) and the quota paid out by check-ins so far.
func GetWtPublicStats(now time.Time) (*WtPublicStats, error) {
	stats := &WtPublicStats{UpdatedAt: now.Unix()}
	if err := DB.Model(&User{}).Where("role < ? AND status = ?", common.RoleAdminUser, common.UserStatusEnabled).
		Count(&stats.Members).Error; err != nil {
		return nil, err
	}
	start, end := wtCallWindow(WtPeriodToday, now)
	if err := LOG_DB.Model(&Log{}).Where("type = ? AND created_at >= ? AND created_at < ?", LogTypeConsume, start, end).
		Count(&stats.CallsToday).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&Checkin{}).Select("COALESCE(SUM(quota_awarded), 0)").Scan(&stats.RewardedQuotaTotal).Error; err != nil {
		return nil, err
	}
	return stats, nil
}
