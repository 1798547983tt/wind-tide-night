package model

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// wind-tide: the call counters behind the member leaderboards (wind-tide/API.md
// §7). Every relay call a member finishes adds to the member's row of the
// Beijing day and moves the member's streaks. The rows outlive the log
// tables, which the server cleans, so the boards keep their history; empty
// counters are filled once from the consume and error logs that still exist.
// One worker writes the counters off the request path, so the database sees
// a single writer and SQLite never has to serialise a crowd of them.

const (
	wtBackfillBatch = 2000
	wtCallQueueSize = 8192
)

// WtCallDay is one member's calls of one Beijing day: Calls succeeded,
// Failed did not.
type WtCallDay struct {
	Id     int    `json:"id"`
	UserId int    `json:"user_id" gorm:"not null;uniqueIndex:idx_wt_call_days_user_day,priority:1"`
	Day    string `json:"day" gorm:"type:varchar(10);not null;uniqueIndex:idx_wt_call_days_user_day,priority:2;index:idx_wt_call_days_day"`
	Calls  int64  `json:"calls" gorm:"not null"`
	Failed int64  `json:"failed" gorm:"not null"`
}

func (WtCallDay) TableName() string {
	return "wt_call_days"
}

// WtCallStreak is one member's runs of calls: the current run of successes,
// the current run of failures (one of the two is always 0) and the longest
// run of each so far.
type WtCallStreak struct {
	Id          int   `json:"id"`
	UserId      int   `json:"user_id" gorm:"not null;uniqueIndex:idx_wt_call_streaks_user"`
	SuccessRun  int64 `json:"success_run" gorm:"not null"`
	FailRun     int64 `json:"fail_run" gorm:"not null"`
	BestSuccess int64 `json:"best_success" gorm:"not null"`
	BestFail    int64 `json:"best_fail" gorm:"not null"`
	UpdatedAt   int64 `json:"updated_at" gorm:"type:bigint;not null"`
}

func (WtCallStreak) TableName() string {
	return "wt_call_streaks"
}

// wtStreakAfter is the member's streak after one more call.
func wtStreakAfter(streak WtCallStreak, ok bool) WtCallStreak {
	if ok {
		streak.SuccessRun++
		streak.FailRun = 0
		streak.BestSuccess = max(streak.BestSuccess, streak.SuccessRun)
		return streak
	}
	streak.FailRun++
	streak.SuccessRun = 0
	streak.BestFail = max(streak.BestFail, streak.FailRun)
	return streak
}

// wtCallEvent is one finished call waiting to be counted.
type wtCallEvent struct {
	userID int
	ok     bool
	at     time.Time
}

var (
	wtCallQueue   = make(chan wtCallEvent, wtCallQueueSize)
	wtCallWorker  sync.Once
	wtCallPending sync.WaitGroup
)

// WtCountCall queues one finished relay call of a member at now for the
// worker to write, in the order the calls finished. A full queue is written
// from a goroutine of its own rather than dropped; the anonymous are not
// counted. A failure to count is logged, never answered.
func WtCountCall(userID int, ok bool, now time.Time) {
	if userID <= 0 {
		return
	}
	wtCallWorker.Do(func() {
		gopool.Go(func() {
			for event := range wtCallQueue {
				wtRecordCallEvent(event)
			}
		})
	})
	event := wtCallEvent{userID: userID, ok: ok, at: now}
	wtCallPending.Add(1)
	select {
	case wtCallQueue <- event:
	default:
		gopool.Go(func() { wtRecordCallEvent(event) })
	}
}

func wtRecordCallEvent(event wtCallEvent) {
	defer wtCallPending.Done()
	if err := WtRecordCall(event.userID, event.ok, event.at); err != nil {
		common.SysError(common.LogText("wt calls: the call of member %d was not counted: %s", event.userID, err.Error()))
	}
}

// WtFlushCallCounters returns once every queued call is written; tests call
// it before they read the counters.
func WtFlushCallCounters() {
	wtCallPending.Wait()
}

// WtRecordCall counts one finished relay call of a member at now: the day row
// and the streak row each move by one atomic update, created on the member's
// first call. GORM writes the columns of a map in name order, so best_fail
// and best_success are computed from the run before it moves, as MySQL
// applies assignments left to right.
func WtRecordCall(userID int, ok bool, now time.Time) error {
	day := WtCheckinDay(now)
	dayMove := map[string]any{"calls": gorm.Expr("calls + 1")}
	if !ok {
		dayMove = map[string]any{"failed": gorm.Expr("failed + 1")}
	}
	if err := wtMoveCounter(&WtCallDay{}, &WtCallDay{UserId: userID, Day: day}, "user_id = ? AND day = ?", []any{userID, day}, dayMove); err != nil {
		return err
	}
	streakMove := map[string]any{
		"best_success": gorm.Expr("CASE WHEN success_run + 1 > best_success THEN success_run + 1 ELSE best_success END"),
		"success_run":  gorm.Expr("success_run + 1"),
		"fail_run":     0,
		"updated_at":   now.Unix(),
	}
	if !ok {
		streakMove = map[string]any{
			"best_fail":   gorm.Expr("CASE WHEN fail_run + 1 > best_fail THEN fail_run + 1 ELSE best_fail END"),
			"fail_run":    gorm.Expr("fail_run + 1"),
			"success_run": 0,
			"updated_at":  now.Unix(),
		}
	}
	return wtMoveCounter(&WtCallStreak{}, &WtCallStreak{UserId: userID, UpdatedAt: now.Unix()}, "user_id = ?", []any{userID}, streakMove)
}

// wtMoveCounter applies the move to the row of table the condition names,
// creating fresh first when the member has no row yet. The insert ignores a
// row another writer created in between, and the move is tried once more.
func wtMoveCounter(table, fresh any, condition string, args []any, move map[string]any) error {
	for attempt := range 2 {
		result := DB.Model(table).Where(condition, args...).Updates(move)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 || attempt == 1 {
			return nil
		}
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(fresh).Error; err != nil {
			return err
		}
	}
	return nil
}

// WtBackfillCallStats fills empty counters from the consume logs (successful
// calls) and error logs (failed calls) the log database still holds, in the
// order they were written. It does nothing once any counter exists, so a
// server that cleaned its logs keeps what the counters already know.
func WtBackfillCallStats() error {
	var existing int64
	if err := DB.Model(&WtCallDay{}).Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}
	days := map[int]map[string]*WtCallDay{}
	streaks := map[int]*WtCallStreak{}
	lastID := 0
	for {
		var rows []Log
		err := LOG_DB.Select("id", "user_id", "type", "created_at").
			Where("id > ? AND type IN ?", lastID, []int{LogTypeConsume, LogTypeError}).
			Order("id ASC").Limit(wtBackfillBatch).Find(&rows).Error
		if err != nil {
			return err
		}
		for _, row := range rows {
			lastID = row.Id
			if row.UserId <= 0 {
				continue
			}
			ok := row.Type == LogTypeConsume
			day := WtCheckinDay(time.Unix(row.CreatedAt, 0))
			byDay := days[row.UserId]
			if byDay == nil {
				byDay = map[string]*WtCallDay{}
				days[row.UserId] = byDay
			}
			entry := byDay[day]
			if entry == nil {
				entry = &WtCallDay{UserId: row.UserId, Day: day}
				byDay[day] = entry
			}
			if ok {
				entry.Calls++
			} else {
				entry.Failed++
			}
			streak := streaks[row.UserId]
			if streak == nil {
				streak = &WtCallStreak{UserId: row.UserId}
				streaks[row.UserId] = streak
			}
			*streak = wtStreakAfter(*streak, ok)
		}
		if len(rows) < wtBackfillBatch {
			break
		}
	}
	now := WtNow().Unix()
	dayRows := make([]WtCallDay, 0, len(days))
	for _, byDay := range days {
		for _, entry := range byDay {
			dayRows = append(dayRows, *entry)
		}
	}
	streakRows := make([]WtCallStreak, 0, len(streaks))
	for _, streak := range streaks {
		streak.UpdatedAt = now
		streakRows = append(streakRows, *streak)
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if len(dayRows) > 0 {
			if err := tx.CreateInBatches(dayRows, 500).Error; err != nil {
				return err
			}
		}
		if len(streakRows) > 0 {
			if err := tx.CreateInBatches(streakRows, 500).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
