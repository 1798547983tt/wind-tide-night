package model

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// wind-tide: the option values wind-tide starts with and the one-time
// takeover of a database (wind-tide/API.md §1, §6, §10 and §11). The values
// are stored as options instead of changing the upstream defaults, and an
// option that already has a value keeps it. Discord-only sign-in is enforced
// in middleware whatever these options say.

const (
	// wtAdoptedAtKey is the option that marks a database wind-tide has taken
	// over, with the unix time as its value. Only its presence counts.
	wtAdoptedAtKey = "WtAdoptedAt"

	// wtAdoptionNote is the note of the removed allowlist entries the
	// takeover records for accounts it disables.
	wtAdoptionNote = "Removed by the wind-tide upgrade"
)

// wtAdopted reports whether wind-tide has taken over the database.
func wtAdopted() (bool, error) {
	var marks int64
	err := DB.Model(&Option{}).Where(commonKeyCol+" = ?", wtAdoptedAtKey).Count(&marks).Error
	return marks > 0, err
}

// WtPrepareDatabase runs on the master node before the setup check. Until
// wind-tide has taken over the database it stores the wind-tide defaults, and
// a database without a root account gets the owner's, so the upstream setup
// wizard never appears. An existing root is bound later by
// WtTakeOverDatabase, once the caches its sessions live in are ready.
func WtPrepareDatabase() error {
	adopted, err := wtAdopted()
	if err != nil {
		return err
	}
	if !adopted {
		checkinReward := strconv.Itoa(int(9999 * common.QuotaPerUnit))
		options := []Option{
			{Key: "SystemName", Value: "风与潮之夜"},
			{Key: "PasswordLoginEnabled", Value: "false"},
			{Key: "PasswordRegisterEnabled", Value: "false"},
			{Key: "RegisterEnabled", Value: "true"},
			{Key: "QuotaForNewUser", Value: "0"},
			{Key: "discord.enabled", Value: "true"},
			{Key: "general_setting.quota_display_type", Value: "USD"},
			{Key: "ModelRequestRateLimitEnabled", Value: "true"},
			{Key: "ModelRequestRateLimitDurationMinutes", Value: "1"},
			{Key: "ModelRequestRateLimitCount", Value: "5"},
			{Key: "checkin_setting.enabled", Value: "true"},
			{Key: "checkin_setting.min_quota", Value: checkinReward},
			{Key: "checkin_setting.max_quota", Value: checkinReward},
			// Visitors see only the opening, home, sign-in and about pages
			// (wind-tide/DESIGN.md §10); the docs link would lead to
			// upstream's docs until the owner sets the site's own.
			{Key: "HeaderNavModules", Value: `{"home":true,"console":true,"pricing":{"enabled":true,"requireAuth":true},"rankings":{"enabled":true,"requireAuth":true},"docs":false,"about":true}`},
		}
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&options).Error; err != nil {
			return err
		}
	}
	return bootstrapWtOwner(false)
}

// WtTakeOverDatabase runs on the master node once the log database and Redis
// are ready. It binds the root account to WT_OWNER_DISCORD_ID and, the first
// time only, disables every account that is not root and has no active
// allowlist entry: a database set up before wind-tide must not keep members
// the owner never admitted. Each account is disabled through the upstream
// user update (its auth version advances), its browser sessions are revoked
// and its cached API tokens dropped. An account with a Discord ID gets a
// removed allowlist entry, so the owner can restore it. The sweep is audited
// with counts only, and the marker is stored last, so a takeover that stopped
// half-way runs again on the next start.
func WtTakeOverDatabase() error {
	if err := WtBootstrapOwner(); err != nil {
		return err
	}
	adopted, err := wtAdopted()
	if err != nil || adopted {
		return err
	}
	now := time.Now().Unix()
	var candidates []User
	if err := DB.Select("id", "discord_id").Where("role <> ?", common.RoleRootUser).Order("id").Find(&candidates).Error; err != nil {
		return err
	}
	disabled, recorded := 0, 0
	for _, candidate := range candidates {
		var account *User
		entryAdded := false
		err := DB.Transaction(func(tx *gorm.DB) error {
			// Allowlist first, then the account: the lock order of removal.
			var entry *WtAllowlistEntry
			if candidate.DiscordId != "" {
				var err error
				if entry, err = findWtAllowlistEntryWithTx(tx, candidate.DiscordId); err != nil {
					return err
				}
				if entry != nil && entry.Status == WtAllowlistActive {
					return nil
				}
			}
			var user User
			if err := lockForUpdate(tx.Where("id = ?", candidate.Id)).Limit(1).Find(&user).Error; err != nil {
				return err
			}
			if user.Id == 0 || user.Role == common.RoleRootUser {
				return nil
			}
			if candidate.DiscordId != "" && entry == nil {
				discordID := candidate.DiscordId
				if err := tx.Create(&WtAllowlistEntry{
					DiscordId: &discordID,
					Note:      wtAdoptionNote,
					Status:    WtAllowlistRemoved,
					UserId:    user.Id,
					CreatedAt: now,
					UpdatedAt: now,
				}).Error; err != nil {
					return err
				}
				entryAdded = true
			}
			user.Status = common.UserStatusDisabled
			if err := user.UpdateWithTx(tx, false); err != nil {
				return err
			}
			account = &user
			return nil
		})
		if err != nil {
			return err
		}
		if account == nil {
			continue
		}
		disabled++
		if entryAdded {
			recorded++
		}
		if err := updateUserCache(*account); err != nil {
			common.SysError(common.LogText("failed to update the cache of user %d: %s", account.Id, err.Error()))
		}
		if _, err := RevokeAllUserSessions(account.Id, "wt_takeover"); err != nil {
			return err
		}
		if err := InvalidateUserTokensCache(account.Id); err != nil {
			common.SysError(common.LogText("failed to invalidate tokens cache for user %d: %s", account.Id, err.Error()))
		}
	}
	if disabled > 0 {
		common.SysLog(common.LogText("wind-tide took over the database: disabled %d accounts outside the allowlist", disabled))
		owner, err := WtOwnerAccount()
		if err != nil {
			return err
		}
		RecordOperationAuditLog(owner.Id, owner.Role,
			"Disabled "+strconv.Itoa(disabled)+" accounts outside the allowlist when wind-tide took over the database",
			"", "wt.takeover", map[string]any{"accounts": disabled, "removed_entries": recorded}, nil, nil)
	}
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&Option{Key: wtAdoptedAtKey, Value: strconv.FormatInt(now, 10)}).Error
}
