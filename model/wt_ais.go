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

// wind-tide: the call pools of the donated-account providers (wind-tide/API.md
// §15): the daily call ledger of every donor on each relay model, the public
// pool of calls they share per model and the announcement feed of every pool.
// The AIS playground pool (aistudio-api) and the BUILD pool (AIStudioToAPI)
// are two providers of the same tables; the Key of a model tells its rows
// apart. Days are Beijing check-in days (WtCheckinDay); nothing carries over.

const (
	// WtAisProvider marks the AI Studio rows of wt_donations and
	// WtBuildProvider the BUILD rows; Antigravity rows carry
	// service.WtDonationProvider.
	WtAisProvider   = "aistudio"
	WtBuildProvider = "build"

	// WtAisMaxPerUser is how many verifying and active donations one member
	// may hold in each of the two pools.
	WtAisMaxPerUser = 10
	// WtPoolEventsKeep is how many announcements the feed keeps.
	WtPoolEventsKeep = 200

	WtPoolEventAisDonated         = "ais_account_donated"
	WtPoolEventAisRemoved         = "ais_account_removed"
	WtPoolEventAisGiven           = "ais_given"
	WtPoolEventAisTaken           = "ais_taken"
	WtPoolEventBuildDonated       = "build_account_donated"
	WtPoolEventBuildRemoved       = "build_account_removed"
	WtPoolEventBuildGiven         = "build_given"
	WtPoolEventBuildTaken         = "build_taken"
	WtPoolEventAntigravityDonated = "antigravity_donated"
	WtPoolEventAntigravityRemoved = "antigravity_removed"

	wtDisplayNameMax = 64
)

// WtAisModelSpec is one relay model of a pool. Key names the model in the
// ledgers, the pools, the events and the API: the upstream name for the AIS
// models, the relay name for the BUILD model, whose upstream the AIS flash
// model shares.
type WtAisModelSpec struct {
	Provider        string // WtAisProvider or WtBuildProvider
	Name            string // the relay model members call, "AIS PG-gemini-4-argon"
	Upstream        string // the model the pool serves, "gemini-4-argon"
	Key             string // the ledger key, unique across the table
	CallsPerAccount int    // what one active donated account earns its donor each day
	TakeMaxPerDay   int    // what one member may take from the public pool in a day
}

// WtAisModels is the model table of both pools, in display order.
var WtAisModels = []WtAisModelSpec{
	{Provider: WtAisProvider, Name: "AIS PG-gemini-4-argon", Upstream: "gemini-4-argon", Key: "gemini-4-argon", CallsPerAccount: 50, TakeMaxPerDay: 100},
	{Provider: WtAisProvider, Name: "AIS PG-gemini-3.8-flash", Upstream: "gemini-3.8-flash", Key: "gemini-3.8-flash", CallsPerAccount: 100, TakeMaxPerDay: 100},
	{Provider: WtBuildProvider, Name: "Build-gemini-3.8-flash", Upstream: "gemini-3.8-flash", Key: "Build-gemini-3.8-flash", CallsPerAccount: 100, TakeMaxPerDay: 100},
}

// WtAisModelsOf is the part of the table one pool serves, in display order.
func WtAisModelsOf(provider string) []WtAisModelSpec {
	specs := make([]WtAisModelSpec, 0, len(WtAisModels))
	for _, spec := range WtAisModels {
		if spec.Provider == provider {
			specs = append(specs, spec)
		}
	}
	return specs
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

// WtAisModelByKey finds the spec of a ledger key, exactly.
func WtAisModelByKey(key string) (*WtAisModelSpec, bool) {
	for _, spec := range WtAisModels {
		if spec.Key == key {
			return &spec, true
		}
	}
	return nil, false
}

var (
	ErrWtAisNotDonor       = common.NewMessage("Donate an AI Studio account first to use the AIS PG pool")
	ErrWtAisExhausted      = common.NewMessage("Your AIS PG calls for today are used up; take some from the pool or wait for tomorrow")
	ErrWtBuildNotDonor     = common.NewMessage("Donate a BUILD account first to use the BUILD pool")
	ErrWtBuildExhausted    = common.NewMessage("Your BUILD calls for today are used up; take some from the pool or wait for tomorrow")
	ErrWtAisPoolEmpty      = common.NewMessage("The pool does not hold that many calls right now")
	ErrWtAisGiveLimit      = common.NewMessage("You can only give calls you still have today")
	ErrWtAisCookie         = common.NewMessage("Paste the Cookie of a signed-in AI Studio page")
	ErrWtAisCount          = common.NewMessage("Enter a whole number of calls, 1 or more")
	ErrWtAisUnknownModel   = common.NewMessage("Unknown AIS PG model")
	ErrWtBuildUnknownModel = common.NewMessage("Unknown BUILD model")
)

// NotDonorError refuses a member without an active account of the model's
// pool.
func (spec *WtAisModelSpec) NotDonorError() *common.Message {
	if spec.Provider == WtBuildProvider {
		return ErrWtBuildNotDonor
	}
	return ErrWtAisNotDonor
}

// ExhaustedError refuses a member whose calls of the day are used up.
func (spec *WtAisModelSpec) ExhaustedError() *common.Message {
	if spec.Provider == WtBuildProvider {
		return ErrWtBuildExhausted
	}
	return ErrWtAisExhausted
}

// givenEvent and takenEvent are the announcement kinds of the model's pool.
func (spec *WtAisModelSpec) givenEvent() string {
	if spec.Provider == WtBuildProvider {
		return WtPoolEventBuildGiven
	}
	return WtPoolEventAisGiven
}

func (spec *WtAisModelSpec) takenEvent() string {
	if spec.Provider == WtBuildProvider {
		return WtPoolEventBuildTaken
	}
	return WtPoolEventAisTaken
}

// WtAisTakeLimitError refuses a take beyond the model's daily cap.
func WtAisTakeLimitError(spec *WtAisModelSpec) *common.Message {
	return common.NewMessage("You can take at most {{max}} calls a day from the pool", map[string]any{"max": spec.TakeMaxPerDay})
}

// wtAisSpec resolves a ledger key or refuses it.
func wtAisSpec(key string) (*WtAisModelSpec, error) {
	spec, ok := WtAisModelByKey(key)
	if !ok {
		return nil, ErrWtAisUnknownModel
	}
	return spec, nil
}

// WtAisLedger is one member's day on one pool model: calls made, calls that
// failed and were refunded, calls given to and taken from the public pool.
// Model is the ledger key of the model.
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
// owner and never sent to the browser. Model is the ledger key of a give or
// take and empty for account events.
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

// WtAisModelBalance is a member's day on one pool model, as the pool page
// shows it. Allowance is CallsPerAccount per active account plus what was
// taken from the pool minus what was given to it. PoolGiven and PoolTaken are
// the day's totals of the public pool, which everybody shares.
type WtAisModelBalance struct {
	Provider    string `json:"provider"`
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
	PoolGiven   int    `json:"pool_given"`
	PoolTaken   int    `json:"pool_taken"`
}

// WtAisBalance is a member's account of one pool for one day: the active
// accounts and one balance per model, in table order.
type WtAisBalance struct {
	Provider string              `json:"provider"`
	Day      string              `json:"day"`
	Accounts int                 `json:"accounts"`
	Models   []WtAisModelBalance `json:"models"`
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

// WtAisFingerprintInPool reports whether a verifying or active donation of
// the provider already carries the cookie fingerprint.
func WtAisFingerprintInPool(provider, fingerprint string) (bool, error) {
	var count int64
	err := DB.Model(&WtDonation{}).Where("provider = ? AND auth_index = ? AND status IN ?", provider, fingerprint, wtDonationSlotStates).Count(&count).Error
	return count > 0, err
}

// CreateWtAisDonation records an imported account of a pool as verifying:
// the pool's account id is the credential name, the cookie fingerprint the
// auth index. The email, when the pool can read one, is learned by the
// verification.
func CreateWtAisDonation(provider string, userID int, accountID, fingerprint string) (*WtDonation, error) {
	now := WtNow().Unix()
	donation := &WtDonation{
		UserId:         userID,
		Provider:       provider,
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

// BindWtAisDonation marks a verifying donation active with the email the
// pool read from the account, "" when the pool reads none.
func BindWtAisDonation(id int, email string) error {
	return wtSettleDonation(id, map[string]any{
		"status":      WtDonationActive,
		"email":       email,
		"verified_at": WtNow().Unix(),
		"fail_reason": "",
	})
}

// wtAisActiveAccounts counts the member's active donations to the provider
// inside tx.
func wtAisActiveAccounts(tx *gorm.DB, userID int, provider string) (int, error) {
	var count int64
	err := tx.Model(&WtDonation{}).Where("user_id = ? AND provider = ? AND status = ?", userID, provider, WtDonationActive).Count(&count).Error
	return int(count), err
}

func wtAisAllowanceOf(spec *WtAisModelSpec, accounts int, ledger *WtAisLedger) int {
	return accounts*spec.CallsPerAccount + ledger.Taken - ledger.Given
}

// wtAisLedgerForUpdate locks the member's ledger row of the day and model
// inside tx, creating it first when the day has none. The plain read before
// the insert keeps MySQL from gap-locking an empty range and deadlocking two
// first calls of the day.
func wtAisLedgerForUpdate(tx *gorm.DB, userID int, day, key string) (*WtAisLedger, error) {
	var count int64
	if err := tx.Model(&WtAisLedger{}).Where("user_id = ? AND day = ? AND model = ?", userID, day, key).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		fresh := WtAisLedger{UserId: userID, Day: day, Model: key, UpdatedAt: WtNow().Unix()}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&fresh).Error; err != nil {
			return nil, err
		}
	}
	var ledger WtAisLedger
	if err := lockForUpdate(tx).Where("user_id = ? AND day = ? AND model = ?", userID, day, key).First(&ledger).Error; err != nil {
		return nil, err
	}
	return &ledger, nil
}

// wtAisPoolForUpdate locks the public pool row of the day and model inside
// tx, creating it first when the day has none.
func wtAisPoolForUpdate(tx *gorm.DB, day, key string) (*WtAisPool, error) {
	var count int64
	if err := tx.Model(&WtAisPool{}).Where("day = ? AND model = ?", day, key).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		fresh := WtAisPool{Day: day, Model: key, UpdatedAt: WtNow().Unix()}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&fresh).Error; err != nil {
			return nil, err
		}
	}
	var pool WtAisPool
	if err := lockForUpdate(tx).Where("day = ? AND model = ?", day, key).First(&pool).Error; err != nil {
		return nil, err
	}
	return &pool, nil
}

// WtAisBalanceOfDay is the member's account of one pool for the day on every
// model of that pool, read without locks; a member without ledger rows has
// made no calls.
func WtAisBalanceOfDay(userID int, day, provider string) (*WtAisBalance, error) {
	return wtAisBalanceOf(userID, day, provider, WtAisModelsOf(provider))
}

// WtAisAllowance is the member's balance of the day on one model.
func WtAisAllowance(userID int, day, key string) (*WtAisModelBalance, error) {
	spec, err := wtAisSpec(key)
	if err != nil {
		return nil, err
	}
	balance, err := wtAisBalanceOf(userID, day, spec.Provider, []WtAisModelSpec{*spec})
	if err != nil {
		return nil, err
	}
	return &balance.Models[0], nil
}

func wtAisBalanceOf(userID int, day, provider string, specs []WtAisModelSpec) (*WtAisBalance, error) {
	accounts, err := wtAisActiveAccounts(DB, userID, provider)
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
	balance := &WtAisBalance{Provider: provider, Day: day, Accounts: accounts, Models: make([]WtAisModelBalance, 0, len(specs))}
	for i := range specs {
		spec := &specs[i]
		ledger := ledgerOf[spec.Key]
		pool := poolOf[spec.Key]
		allowance := wtAisAllowanceOf(spec, accounts, &ledger)
		balance.Models = append(balance.Models, WtAisModelBalance{
			Provider:    spec.Provider,
			Model:       spec.Key,
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
			PoolBalance: pool.Balance,
			PoolGiven:   pool.GivenTotal,
			PoolTaken:   pool.TakenTotal,
		})
	}
	return balance, nil
}

// WtAisReserveCall counts one call of the model against the member's day and
// returns how many are left. A member without an active account of the
// model's pool gets the pool's NotDonorError, one at the allowance its
// ExhaustedError.
func WtAisReserveCall(userID int, day, key string) (int, error) {
	spec, err := wtAisSpec(key)
	if err != nil {
		return 0, err
	}
	remaining := 0
	err = DB.Transaction(func(tx *gorm.DB) error {
		ledger, err := wtAisLedgerForUpdate(tx, userID, day, spec.Key)
		if err != nil {
			return err
		}
		accounts, err := wtAisActiveAccounts(tx, userID, spec.Provider)
		if err != nil {
			return err
		}
		if accounts == 0 {
			return spec.NotDonorError()
		}
		allowance := wtAisAllowanceOf(spec, accounts, ledger)
		if ledger.Calls >= allowance {
			return spec.ExhaustedError()
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
func WtAisRefundCall(userID int, day, key string) error {
	spec, err := wtAisSpec(key)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		ledger, err := wtAisLedgerForUpdate(tx, userID, day, spec.Key)
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
func WtAisGive(userID int, day, key string, count int, displayName string) (int, int, error) {
	spec, err := wtAisSpec(key)
	if err != nil {
		return 0, 0, err
	}
	if count < 1 {
		return 0, 0, ErrWtAisCount
	}
	var remaining, balance int
	err = DB.Transaction(func(tx *gorm.DB) error {
		ledger, err := wtAisLedgerForUpdate(tx, userID, day, spec.Key)
		if err != nil {
			return err
		}
		accounts, err := wtAisActiveAccounts(tx, userID, spec.Provider)
		if err != nil {
			return err
		}
		if accounts == 0 {
			return spec.NotDonorError()
		}
		allowance := wtAisAllowanceOf(spec, accounts, ledger)
		if count > allowance-ledger.Calls {
			return ErrWtAisGiveLimit
		}
		pool, err := wtAisPoolForUpdate(tx, day, spec.Key)
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
		return wtInsertPoolEvent(tx, spec.givenEvent(), spec.Provider, userID, displayName, count, spec.Key)
	})
	return remaining, balance, err
}

// WtAisTake moves count calls from the model's public pool to the member's
// day and announces it. It returns the member's remaining calls, the pool's
// balance and what the member may still take today.
func WtAisTake(userID int, day, key string, count int, displayName string) (int, int, int, error) {
	spec, err := wtAisSpec(key)
	if err != nil {
		return 0, 0, 0, err
	}
	if count < 1 {
		return 0, 0, 0, ErrWtAisCount
	}
	var remaining, balance, takeLeft int
	err = DB.Transaction(func(tx *gorm.DB) error {
		ledger, err := wtAisLedgerForUpdate(tx, userID, day, spec.Key)
		if err != nil {
			return err
		}
		accounts, err := wtAisActiveAccounts(tx, userID, spec.Provider)
		if err != nil {
			return err
		}
		if accounts == 0 {
			return spec.NotDonorError()
		}
		if count > spec.TakeMaxPerDay || ledger.Taken+count > spec.TakeMaxPerDay {
			return WtAisTakeLimitError(spec)
		}
		pool, err := wtAisPoolForUpdate(tx, day, spec.Key)
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
		return wtInsertPoolEvent(tx, spec.takenEvent(), spec.Provider, userID, displayName, count, spec.Key)
	})
	return remaining, balance, takeLeft, err
}

// WtAisPoolBalance is what the model's public pool holds today; 0 before the
// first gift of the day.
func WtAisPoolBalance(day, key string) (int, error) {
	spec, err := wtAisSpec(key)
	if err != nil {
		return 0, err
	}
	var pool WtAisPool
	err = DB.Where("day = ? AND model = ?", day, spec.Key).First(&pool).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return pool.Balance, err
}

// WtInsertPoolEvent adds an announcement to the feed and drops everything
// older than the newest WtPoolEventsKeep. model is the ledger key of a give
// or take, "" for account events.
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
