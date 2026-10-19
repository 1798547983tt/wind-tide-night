package model

import (
	"cmp"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// wind-tide: the AIS playground pool (wind-tide/API.md §15): the daily call
// ledger of every donor of an AI Studio account on each relay model, the
// public pool of calls they share per model, the announcement feed of both
// pools and the leaderboards. Days are Beijing check-in days (WtCheckinDay);
// nothing carries over.

const (
	// WtAisProvider marks the AI Studio rows of wt_donations; Antigravity
	// rows carry service.WtDonationProvider.
	WtAisProvider = "aistudio"

	// WtAisMaxPerUser is how many verifying and active AI Studio donations one
	// member may hold.
	WtAisMaxPerUser = 10
	// WtPoolEventsKeep is how many announcements the feed keeps.
	WtPoolEventsKeep = 200

	WtPoolEventAisDonated         = "ais_account_donated"
	WtPoolEventAisRemoved         = "ais_account_removed"
	WtPoolEventAisGiven           = "ais_given"
	WtPoolEventAisTaken           = "ais_taken"
	WtPoolEventAntigravityDonated = "antigravity_donated"
	WtPoolEventAntigravityRemoved = "antigravity_removed"

	wtDisplayNameMax = 64
)

// WtAisModelSpec is one relay model of the AIS PG group. Upstream is the model
// aistudio-api serves and the key the ledgers, the pools, the events and the
// API use for it.
type WtAisModelSpec struct {
	Name            string // the relay model members call, "AIS PG-gemini-4-argon"
	Upstream        string // the upstream model, "gemini-4-argon"
	CallsPerAccount int    // what one active donated account earns its donor each day
	TakeMaxPerDay   int    // what one member may take from the public pool in a day
}

// WtAisModels is the model table of the pool, in display order.
var WtAisModels = []WtAisModelSpec{
	{Name: "AIS PG-gemini-4-argon", Upstream: "gemini-4-argon", CallsPerAccount: 50, TakeMaxPerDay: 100},
	{Name: "AIS PG-gemini-3.8-flash", Upstream: "gemini-3.8-flash", CallsPerAccount: 100, TakeMaxPerDay: 100},
}

// WtAisModelByName finds the spec of a relay model name, exactly.
func WtAisModelByName(relayName string) (*WtAisModelSpec, bool) {
	for _, spec := range WtAisModels {
		if spec.Name == relayName {
			return &spec, true
		}
	}
	return nil, false
}

// WtAisModelByUpstream finds the spec of an upstream model key, exactly.
func WtAisModelByUpstream(upstream string) (*WtAisModelSpec, bool) {
	for _, spec := range WtAisModels {
		if spec.Upstream == upstream {
			return &spec, true
		}
	}
	return nil, false
}

var (
	ErrWtAisNotDonor     = common.NewMessage("Donate an AI Studio account first to use the AIS PG pool")
	ErrWtAisExhausted    = common.NewMessage("Your AIS PG calls for today are used up; take some from the pool or wait for tomorrow")
	ErrWtAisPoolEmpty    = common.NewMessage("The pool does not hold that many calls right now")
	ErrWtAisGiveLimit    = common.NewMessage("You can only give calls you still have today")
	ErrWtAisCookie       = common.NewMessage("Paste the Cookie of a signed-in AI Studio page")
	ErrWtAisCount        = common.NewMessage("Enter a whole number of calls, 1 or more")
	ErrWtAisUnknownModel = common.NewMessage("Unknown AIS PG model")
)

// WtAisTakeLimitError refuses a take beyond the model's daily cap.
func WtAisTakeLimitError(spec *WtAisModelSpec) *common.Message {
	return common.NewMessage("You can take at most {{max}} calls a day from the pool", map[string]any{"max": spec.TakeMaxPerDay})
}

// wtAisSpec resolves an upstream model key or refuses it.
func wtAisSpec(upstream string) (*WtAisModelSpec, error) {
	spec, ok := WtAisModelByUpstream(upstream)
	if !ok {
		return nil, ErrWtAisUnknownModel
	}
	return spec, nil
}

// WtAisLedger is one member's AIS PG day on one model: calls made, calls that
// failed and were refunded, calls given to and taken from the public pool.
// Model is the upstream model key.
type WtAisLedger struct {
	Id        int    `json:"id"`
	UserId    int    `json:"user_id" gorm:"not null;index:idx_wt_ais_ledgers_user_id;uniqueIndex:idx_wt_ais_ledgers_user_day_model,priority:1"`
	Day       string `json:"day" gorm:"type:varchar(10);not null;uniqueIndex:idx_wt_ais_ledgers_user_day_model,priority:2"`
	Model     string `json:"model" gorm:"type:varchar(64);not null;uniqueIndex:idx_wt_ais_ledgers_user_day_model,priority:3"`
	Calls     int    `json:"calls" gorm:"not null"`
	Failed    int    `json:"failed" gorm:"not null"`
	Given     int    `json:"given" gorm:"not null"`
	Taken     int    `json:"taken" gorm:"not null"`
	UpdatedAt int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (WtAisLedger) TableName() string {
	return "wt_ais_ledgers"
}

// WtAisPool is the public pool of one day on one model.
type WtAisPool struct {
	Id         int    `json:"id"`
	Day        string `json:"day" gorm:"type:varchar(10);not null;uniqueIndex:idx_wt_ais_pools_day_model,priority:1"`
	Model      string `json:"model" gorm:"type:varchar(64);not null;uniqueIndex:idx_wt_ais_pools_day_model,priority:2"`
	Balance    int    `json:"balance" gorm:"not null"`
	GivenTotal int    `json:"given_total" gorm:"not null"`
	TakenTotal int    `json:"taken_total" gorm:"not null"`
	UpdatedAt  int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (WtAisPool) TableName() string {
	return "wt_ais_pools"
}

// WtPoolEvent is one announcement of the feed. DisplayName is the member's
// name as it was when the event happened; the member id is kept for the
// owner and never sent to the browser. Model is the upstream model of a give
// or take and empty for account events.
type WtPoolEvent struct {
	Id          int    `json:"id"`
	Kind        string `json:"kind" gorm:"type:varchar(32);not null;index:idx_wt_pool_events_kind"`
	Provider    string `json:"provider" gorm:"type:varchar(16);not null"`
	UserId      int    `json:"-" gorm:"not null;index:idx_wt_pool_events_user_id"`
	DisplayName string `json:"display_name" gorm:"type:varchar(64);not null"`
	Amount      int    `json:"amount" gorm:"not null"`
	Model       string `json:"model" gorm:"type:varchar(64);not null"`
	CreatedAt   int64  `json:"created_at" gorm:"type:bigint;not null;index:idx_wt_pool_events_created_at"`
}

func (WtPoolEvent) TableName() string {
	return "wt_pool_events"
}

// WtAisModelBalance is a member's day on one AIS PG model, as the pool page
// shows it. Allowance is CallsPerAccount per active account plus what was
// taken from the pool minus what was given to it.
type WtAisModelBalance struct {
	Model       string `json:"model"`
	RelayModel  string `json:"relay_model"`
	PerAccount  int    `json:"per_account"`
	TakeMax     int    `json:"take_max"`
	Allowance   int    `json:"allowance"`
	Calls       int    `json:"calls"`
	FailedToday int    `json:"failed_today"`
	Given       int    `json:"given"`
	Taken       int    `json:"taken"`
	Remaining   int    `json:"remaining"`
	TakeLeft    int    `json:"take_left"`
	PoolBalance int    `json:"pool_balance"`
}

// WtAisBalance is a member's AIS PG account for one day: the active accounts
// and one balance per model, in table order.
type WtAisBalance struct {
	Day      string              `json:"day"`
	Accounts int                 `json:"accounts"`
	Models   []WtAisModelBalance `json:"models"`
}

// WtAisLeaderboardEntry is one row of a leaderboard.
type WtAisLeaderboardEntry struct {
	Rank        int    `json:"rank"`
	DisplayName string `json:"display_name"`
	Count       int    `json:"count"`
}

// WtAisLeaderboardBoard is the two boards of one period.
type WtAisLeaderboardBoard struct {
	Calls  []WtAisLeaderboardEntry `json:"calls"`
	Failed []WtAisLeaderboardEntry `json:"failed"`
}

// WtAisModelLeaderboard is the AIS PG leaderboard of one model, today and of
// all time.
type WtAisModelLeaderboard struct {
	Model      string                `json:"model"`
	RelayModel string                `json:"relay_model"`
	Today      WtAisLeaderboardBoard `json:"today"`
	Total      WtAisLeaderboardBoard `json:"total"`
}

// WtDisplayNameOf is the name announcements and leaderboards show for a
// member: the display name, else the username, at most 64 runes.
func WtDisplayNameOf(user *User) string {
	if user == nil {
		return ""
	}
	return wtTruncateDisplayName(cmp.Or(strings.TrimSpace(user.DisplayName), user.Username))
}

// WtDisplayNameOfUser loads the member and returns WtDisplayNameOf; "" when
// the member cannot be loaded.
func WtDisplayNameOfUser(userID int) string {
	user, err := GetUserById(userID, false)
	if err != nil {
		return ""
	}
	return WtDisplayNameOf(user)
}

func wtTruncateDisplayName(name string) string {
	if utf8.RuneCountInString(name) > wtDisplayNameMax {
		return string([]rune(name)[:wtDisplayNameMax])
	}
	return name
}

// WtAisFingerprintInPool reports whether a verifying or active AI Studio
// donation already carries the cookie fingerprint.
func WtAisFingerprintInPool(fingerprint string) (bool, error) {
	var count int64
	err := DB.Model(&WtDonation{}).Where("provider = ? AND auth_index = ? AND status IN ?", WtAisProvider, fingerprint, wtDonationSlotStates).Count(&count).Error
	return count > 0, err
}

// CreateWtAisDonation records an imported AI Studio account as verifying:
// the pool's account id is the credential name, the cookie fingerprint the
// auth index. The email is learned by the verification.
func CreateWtAisDonation(userID int, accountID, fingerprint string) (*WtDonation, error) {
	now := WtNow().Unix()
	donation := &WtDonation{
		UserId:         userID,
		Provider:       WtAisProvider,
		CredentialName: accountID,
		AuthIndex:      fingerprint,
		Status:         WtDonationVerifying,
		StartedAt:      now,
		SubmittedAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := DB.Create(donation).Error; err != nil {
		return nil, err
	}
	return donation, nil
}

// BindWtAisDonation marks a verifying AI Studio donation active with the
// email the pool read from the account.
func BindWtAisDonation(id int, email string) error {
	return wtSettleDonation(id, map[string]any{
		"status":      WtDonationActive,
		"email":       email,
		"verified_at": WtNow().Unix(),
		"fail_reason": "",
	})
}

// wtAisActiveAccounts counts the member's active AI Studio donations inside tx.
func wtAisActiveAccounts(tx *gorm.DB, userID int) (int, error) {
	var count int64
	err := tx.Model(&WtDonation{}).Where("user_id = ? AND provider = ? AND status = ?", userID, WtAisProvider, WtDonationActive).Count(&count).Error
	return int(count), err
}

func wtAisAllowanceOf(spec *WtAisModelSpec, accounts int, ledger *WtAisLedger) int {
	return accounts*spec.CallsPerAccount + ledger.Taken - ledger.Given
}

// wtAisLedgerForUpdate locks the member's ledger row of the day and model
// inside tx, creating it first when the day has none. The plain read before
// the insert keeps MySQL from gap-locking an empty range and deadlocking two
// first calls of the day.
func wtAisLedgerForUpdate(tx *gorm.DB, userID int, day, upstream string) (*WtAisLedger, error) {
	var count int64
	if err := tx.Model(&WtAisLedger{}).Where("user_id = ? AND day = ? AND model = ?", userID, day, upstream).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		fresh := WtAisLedger{UserId: userID, Day: day, Model: upstream, UpdatedAt: WtNow().Unix()}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&fresh).Error; err != nil {
			return nil, err
		}
	}
	var ledger WtAisLedger
	if err := lockForUpdate(tx).Where("user_id = ? AND day = ? AND model = ?", userID, day, upstream).First(&ledger).Error; err != nil {
		return nil, err
	}
	return &ledger, nil
}

// wtAisPoolForUpdate locks the public pool row of the day and model inside
// tx, creating it first when the day has none.
func wtAisPoolForUpdate(tx *gorm.DB, day, upstream string) (*WtAisPool, error) {
	var count int64
	if err := tx.Model(&WtAisPool{}).Where("day = ? AND model = ?", day, upstream).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		fresh := WtAisPool{Day: day, Model: upstream, UpdatedAt: WtNow().Unix()}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&fresh).Error; err != nil {
			return nil, err
		}
	}
	var pool WtAisPool
	if err := lockForUpdate(tx).Where("day = ? AND model = ?", day, upstream).First(&pool).Error; err != nil {
		return nil, err
	}
	return &pool, nil
}

// WtAisBalanceOfDay is the member's AIS PG account of the day on every model,
// read without locks; a member without ledger rows has made no calls.
func WtAisBalanceOfDay(userID int, day string) (*WtAisBalance, error) {
	return wtAisBalanceOf(userID, day, WtAisModels)
}

// WtAisAllowance is the member's balance of the day on one model.
func WtAisAllowance(userID int, day, upstream string) (*WtAisModelBalance, error) {
	spec, err := wtAisSpec(upstream)
	if err != nil {
		return nil, err
	}
	balance, err := wtAisBalanceOf(userID, day, []WtAisModelSpec{*spec})
	if err != nil {
		return nil, err
	}
	return &balance.Models[0], nil
}

func wtAisBalanceOf(userID int, day string, specs []WtAisModelSpec) (*WtAisBalance, error) {
	accounts, err := wtAisActiveAccounts(DB, userID)
	if err != nil {
		return nil, err
	}
	var ledgers []WtAisLedger
	if err := DB.Where("user_id = ? AND day = ?", userID, day).Find(&ledgers).Error; err != nil {
		return nil, err
	}
	var pools []WtAisPool
	if err := DB.Where("day = ?", day).Find(&pools).Error; err != nil {
		return nil, err
	}
	ledgerOf := make(map[string]WtAisLedger, len(ledgers))
	for _, ledger := range ledgers {
		ledgerOf[ledger.Model] = ledger
	}
	poolOf := make(map[string]WtAisPool, len(pools))
	for _, pool := range pools {
		poolOf[pool.Model] = pool
	}
	balance := &WtAisBalance{Day: day, Accounts: accounts, Models: make([]WtAisModelBalance, 0, len(specs))}
	for i := range specs {
		spec := &specs[i]
		ledger := ledgerOf[spec.Upstream]
		allowance := wtAisAllowanceOf(spec, accounts, &ledger)
		balance.Models = append(balance.Models, WtAisModelBalance{
			Model:       spec.Upstream,
			RelayModel:  spec.Name,
			PerAccount:  spec.CallsPerAccount,
			TakeMax:     spec.TakeMaxPerDay,
			Allowance:   allowance,
			Calls:       ledger.Calls,
			FailedToday: ledger.Failed,
			Given:       ledger.Given,
			Taken:       ledger.Taken,
			Remaining:   max(allowance-ledger.Calls, 0),
			TakeLeft:    max(spec.TakeMaxPerDay-ledger.Taken, 0),
			PoolBalance: poolOf[spec.Upstream].Balance,
		})
	}
	return balance, nil
}

// WtAisReserveCall counts one call of the model against the member's day and
// returns how many are left. A member without an active account gets
// ErrWtAisNotDonor, one at the allowance ErrWtAisExhausted.
func WtAisReserveCall(userID int, day, upstream string) (int, error) {
	spec, err := wtAisSpec(upstream)
	if err != nil {
		return 0, err
	}
	remaining := 0
	err = DB.Transaction(func(tx *gorm.DB) error {
		ledger, err := wtAisLedgerForUpdate(tx, userID, day, spec.Upstream)
		if err != nil {
			return err
		}
		accounts, err := wtAisActiveAccounts(tx, userID)
		if err != nil {
			return err
		}
		if accounts == 0 {
			return ErrWtAisNotDonor
		}
		allowance := wtAisAllowanceOf(spec, accounts, ledger)
		if ledger.Calls >= allowance {
			return ErrWtAisExhausted
		}
		remaining = allowance - ledger.Calls - 1
		return tx.Model(&WtAisLedger{}).Where("id = ?", ledger.Id).Updates(map[string]any{
			"calls":      ledger.Calls + 1,
			"updated_at": WtNow().Unix(),
		}).Error
	})
	return remaining, err
}

// WtAisRefundCall gives a reserved call of the model back after the upstream
// request failed and counts the failure.
func WtAisRefundCall(userID int, day, upstream string) error {
	spec, err := wtAisSpec(upstream)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		ledger, err := wtAisLedgerForUpdate(tx, userID, day, spec.Upstream)
		if err != nil {
			return err
		}
		return tx.Model(&WtAisLedger{}).Where("id = ?", ledger.Id).Updates(map[string]any{
			"calls":      max(ledger.Calls-1, 0),
			"failed":     ledger.Failed + 1,
			"updated_at": WtNow().Unix(),
		}).Error
	})
}

// WtAisGive moves count of the member's remaining calls of the day on the
// model into that model's public pool and announces it. It returns the
// member's remaining calls and the pool's balance.
func WtAisGive(userID int, day, upstream string, count int, displayName string) (int, int, error) {
	spec, err := wtAisSpec(upstream)
	if err != nil {
		return 0, 0, err
	}
	if count < 1 {
		return 0, 0, ErrWtAisCount
	}
	var remaining, balance int
	err = DB.Transaction(func(tx *gorm.DB) error {
		ledger, err := wtAisLedgerForUpdate(tx, userID, day, spec.Upstream)
		if err != nil {
			return err
		}
		accounts, err := wtAisActiveAccounts(tx, userID)
		if err != nil {
			return err
		}
		if accounts == 0 {
			return ErrWtAisNotDonor
		}
		allowance := wtAisAllowanceOf(spec, accounts, ledger)
		if count > allowance-ledger.Calls {
			return ErrWtAisGiveLimit
		}
		pool, err := wtAisPoolForUpdate(tx, day, spec.Upstream)
		if err != nil {
			return err
		}
		now := WtNow().Unix()
		if err := tx.Model(&WtAisLedger{}).Where("id = ?", ledger.Id).Updates(map[string]any{"given": ledger.Given + count, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&WtAisPool{}).Where("id = ?", pool.Id).Updates(map[string]any{"balance": pool.Balance + count, "given_total": pool.GivenTotal + count, "updated_at": now}).Error; err != nil {
			return err
		}
		remaining = allowance - count - ledger.Calls
		balance = pool.Balance + count
		return wtInsertPoolEvent(tx, WtPoolEventAisGiven, WtAisProvider, userID, displayName, count, spec.Upstream)
	})
	return remaining, balance, err
}

// WtAisTake moves count calls from the model's public pool to the member's
// day and announces it. It returns the member's remaining calls, the pool's
// balance and what the member may still take today.
func WtAisTake(userID int, day, upstream string, count int, displayName string) (int, int, int, error) {
	spec, err := wtAisSpec(upstream)
	if err != nil {
		return 0, 0, 0, err
	}
	if count < 1 {
		return 0, 0, 0, ErrWtAisCount
	}
	var remaining, balance, takeLeft int
	err = DB.Transaction(func(tx *gorm.DB) error {
		ledger, err := wtAisLedgerForUpdate(tx, userID, day, spec.Upstream)
		if err != nil {
			return err
		}
		accounts, err := wtAisActiveAccounts(tx, userID)
		if err != nil {
			return err
		}
		if accounts == 0 {
			return ErrWtAisNotDonor
		}
		if count > spec.TakeMaxPerDay || ledger.Taken+count > spec.TakeMaxPerDay {
			return WtAisTakeLimitError(spec)
		}
		pool, err := wtAisPoolForUpdate(tx, day, spec.Upstream)
		if err != nil {
			return err
		}
		if pool.Balance < count {
			return ErrWtAisPoolEmpty
		}
		now := WtNow().Unix()
		if err := tx.Model(&WtAisLedger{}).Where("id = ?", ledger.Id).Updates(map[string]any{"taken": ledger.Taken + count, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&WtAisPool{}).Where("id = ?", pool.Id).Updates(map[string]any{"balance": pool.Balance - count, "taken_total": pool.TakenTotal + count, "updated_at": now}).Error; err != nil {
			return err
		}
		remaining = wtAisAllowanceOf(spec, accounts, ledger) + count - ledger.Calls
		balance = pool.Balance - count
		takeLeft = spec.TakeMaxPerDay - ledger.Taken - count
		return wtInsertPoolEvent(tx, WtPoolEventAisTaken, WtAisProvider, userID, displayName, count, spec.Upstream)
	})
	return remaining, balance, takeLeft, err
}

// WtAisPoolBalance is what the model's public pool holds today; 0 before the
// first gift of the day.
func WtAisPoolBalance(day, upstream string) (int, error) {
	spec, err := wtAisSpec(upstream)
	if err != nil {
		return 0, err
	}
	var pool WtAisPool
	err = DB.Where("day = ? AND model = ?", day, spec.Upstream).First(&pool).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return pool.Balance, err
}

// WtAisLeaderboard is the top limit members by calls and by failed calls on
// one model, today and of all time; members with a zero count are left out.
func WtAisLeaderboard(day, upstream string, limit int) (*WtAisModelLeaderboard, error) {
	spec, err := wtAisSpec(upstream)
	if err != nil {
		return nil, err
	}
	board := &WtAisModelLeaderboard{Model: spec.Upstream, RelayModel: spec.Name}
	for _, target := range []struct {
		entries *[]WtAisLeaderboardEntry
		column  string
		day     string
	}{
		{&board.Today.Calls, "calls", day},
		{&board.Today.Failed, "failed", day},
		{&board.Total.Calls, "calls", ""},
		{&board.Total.Failed, "failed", ""},
	} {
		*target.entries, err = wtAisTopLedgers(target.column, target.day, spec.Upstream, limit)
		if err != nil {
			return nil, err
		}
	}
	return board, nil
}

// WtAisLeaderboards is WtAisLeaderboard for every model, in table order.
func WtAisLeaderboards(day string, limit int) ([]WtAisModelLeaderboard, error) {
	boards := make([]WtAisModelLeaderboard, 0, len(WtAisModels))
	for _, spec := range WtAisModels {
		board, err := WtAisLeaderboard(day, spec.Upstream, limit)
		if err != nil {
			return nil, err
		}
		boards = append(boards, *board)
	}
	return boards, nil
}

// wtAisTopLedgers sums one ledger column of one model per member, newest
// names from the users table, largest first. column is a fixed column name,
// never input.
func wtAisTopLedgers(column, day, upstream string, limit int) ([]WtAisLeaderboardEntry, error) {
	var rows []struct {
		UserId      int
		Total       int
		DisplayName string
		Username    string
	}
	query := DB.Table("wt_ais_ledgers").
		Select("wt_ais_ledgers.user_id AS user_id, SUM(wt_ais_ledgers."+column+") AS total, COALESCE(users.display_name, '') AS display_name, users.username AS username").
		Joins("JOIN users ON users.id = wt_ais_ledgers.user_id").
		Where("wt_ais_ledgers.model = ?", upstream)
	if day != "" {
		query = query.Where("wt_ais_ledgers.day = ?", day)
	}
	err := query.Group("wt_ais_ledgers.user_id, users.display_name, users.username").
		Having("SUM(wt_ais_ledgers." + column + ") > 0").
		Order("total DESC, wt_ais_ledgers.user_id ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	entries := make([]WtAisLeaderboardEntry, 0, len(rows))
	for i, row := range rows {
		entries = append(entries, WtAisLeaderboardEntry{
			Rank:        i + 1,
			DisplayName: wtTruncateDisplayName(cmp.Or(strings.TrimSpace(row.DisplayName), row.Username)),
			Count:       row.Total,
		})
	}
	return entries, nil
}

// WtInsertPoolEvent adds an announcement to the feed and drops everything
// older than the newest WtPoolEventsKeep. model is the upstream model of a
// give or take, "" for account events.
func WtInsertPoolEvent(kind, provider string, userID int, displayName string, amount int, model string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return wtInsertPoolEvent(tx, kind, provider, userID, displayName, amount, model)
	})
}

func wtInsertPoolEvent(tx *gorm.DB, kind, provider string, userID int, displayName string, amount int, model string) error {
	event := &WtPoolEvent{
		Kind:        kind,
		Provider:    provider,
		UserId:      userID,
		DisplayName: wtTruncateDisplayName(displayName),
		Amount:      amount,
		Model:       model,
		CreatedAt:   WtNow().Unix(),
	}
	if err := tx.Create(event).Error; err != nil {
		return err
	}
	// The id of the oldest event to keep, then everything below it: two plain
	// statements, which every dialect runs the same way.
	var keep []int
	if err := tx.Model(&WtPoolEvent{}).Order("id DESC").Offset(WtPoolEventsKeep-1).Limit(1).Pluck("id", &keep).Error; err != nil {
		return err
	}
	if len(keep) == 0 {
		return nil
	}
	return tx.Where("id < ?", keep[0]).Delete(&WtPoolEvent{}).Error
}

// WtListPoolEvents is the feed, newest first, at most limit entries; with
// afterID set only the entries newer than it.
func WtListPoolEvents(afterID, limit int) ([]*WtPoolEvent, error) {
	events := []*WtPoolEvent{}
	query := DB.Order("id DESC").Limit(limit)
	if afterID > 0 {
		query = query.Where("id > ?", afterID)
	}
	err := query.Find(&events).Error
	return events, err
}
