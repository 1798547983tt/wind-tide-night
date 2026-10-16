package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"gorm.io/gorm"
)

// wind-tide: the owner account bound to WT_OWNER_DISCORD_ID and the emergency
// login token (wind-tide/API.md §1 and §4).

const (
	// WtEmergencyTokenTTL is how long an emergency login link stays valid.
	WtEmergencyTokenTTL = 10 * time.Minute

	wtEmergencyTokenBytes = 32
	wtOwnerInitialQuota   = 100000000
)

var (
	ErrWtOwnerNotReady         = common.NewMessage("The owner account is not ready")
	ErrWtEmergencyTokenInvalid = common.NewMessage("Emergency login link is invalid or has expired")

	errWtOwnerDiscordIDTaken = errors.New("WT_OWNER_DISCORD_ID is bound to another account; unbind or delete that account first")
)

// wtOwnerDiscordIDPattern is the shape of a Discord user ID (a snowflake).
var wtOwnerDiscordIDPattern = regexp.MustCompile(`^[0-9]{17,20}$`)

// WtEmergencyToken stores the SHA-256 digest of an emergency login token,
// never the token itself.
type WtEmergencyToken struct {
	Id        int    `json:"id"`
	TokenHash string `json:"-" gorm:"type:varchar(64);not null;uniqueIndex:idx_wt_emergency_tokens_hash"`
	ExpiresAt int64  `json:"expires_at" gorm:"type:bigint;not null"`
	UsedAt    int64  `json:"used_at" gorm:"type:bigint;not null"`
	CreatedAt int64  `json:"created_at" gorm:"type:bigint;not null"`
}

func (WtEmergencyToken) TableName() string {
	return "wt_emergency_tokens"
}

// WtOwnerDiscordID is the owner's Discord user ID from WT_OWNER_DISCORD_ID.
// It is read on every call because .env is loaded after package init.
func WtOwnerDiscordID() string {
	return strings.TrimSpace(os.Getenv("WT_OWNER_DISCORD_ID"))
}

// WtCheckOwnerDiscordID refuses a missing or malformed WT_OWNER_DISCORD_ID.
// Without the owner the gate has no root account to admit, so neither the
// server nor the `wt` commands run.
func WtCheckOwnerDiscordID() error {
	ownerID := WtOwnerDiscordID()
	if ownerID == "" {
		return errors.New("WT_OWNER_DISCORD_ID is not set; set it to the owner's Discord user ID")
	}
	if !wtOwnerDiscordIDPattern.MatchString(ownerID) {
		return errors.New("WT_OWNER_DISCORD_ID must be a Discord user ID of 17 to 20 digits")
	}
	return nil
}

// WtBootstrapOwner makes sure the root account is bound to
// WT_OWNER_DISCORD_ID. Without a root, one is created with no password, which
// leaves Discord as its sign-in method and lets the setup check mark the site
// initialized. A root that is unbound or bound to an older ID is rebound: the
// account then belongs to another Discord account, so nothing the previous
// holder set up may open it any more. In the same transaction its auth
// version advances, its access tokens, passkeys, two-factor setup and
// password are deleted and its API keys are switched off; then its browser
// sessions are revoked and the rebind is audited with counts only. It fails
// when another account holds the ID.
func WtBootstrapOwner() error {
	return bootstrapWtOwner(true)
}

// WtRevokedCredentials counts the credentials an owner rebind or an allowlist
// removal took away from an account; audit logs record these counts only.
type WtRevokedCredentials struct {
	// AccessTokens counts the deleted dashboard access tokens, the legacy
	// access token included.
	AccessTokens int64
	// RelayKeys counts the API keys switched off. They are kept, so their
	// owner still sees them and can switch them on again.
	RelayKeys int64
	Passkeys  int64
	TwoFactor int64
}

// revokeWtAccessWithTx deletes the dashboard access tokens of an account and
// switches its enabled API keys off.
func revokeWtAccessWithTx(tx *gorm.DB, userID int) (WtRevokedCredentials, error) {
	var revoked WtRevokedCredentials
	accessTokens, err := DeleteUserAccessTokensWithTx(tx, userID)
	if err != nil {
		return revoked, err
	}
	legacy := tx.Model(&User{}).Where("id = ? AND access_token IS NOT NULL", userID).
		Updates(map[string]any{"access_token": nil, "access_token_created_at": nil})
	if legacy.Error != nil {
		return revoked, legacy.Error
	}
	keys := tx.Model(&Token{}).Where("user_id = ? AND status = ?", userID, common.TokenStatusEnabled).
		Update("status", common.TokenStatusDisabled)
	if keys.Error != nil {
		return revoked, keys.Error
	}
	revoked.AccessTokens, revoked.RelayKeys = accessTokens+legacy.RowsAffected, keys.RowsAffected
	return revoked, nil
}

// bootstrapWtOwner creates the owner's root account when there is none. Only
// with bindExisting does it rebind an existing root, which needs the caches,
// so the startup call before Redis is ready leaves an existing root alone.
func bootstrapWtOwner(bindExisting bool) error {
	if err := WtCheckOwnerDiscordID(); err != nil {
		return err
	}
	ownerID := WtOwnerDiscordID()
	rebound := 0
	var revoked WtRevokedCredentials
	err := DB.Transaction(func(tx *gorm.DB) error {
		var root User
		if err := lockForUpdate(tx.Where("role = ?", common.RoleRootUser)).Order("id").Limit(1).Find(&root).Error; err != nil {
			return err
		}
		if root.Id != 0 && (root.DiscordId == ownerID || !bindExisting) {
			return nil
		}
		var holders int64
		if err := tx.Unscoped().Model(&User{}).Where("discord_id = ? AND id <> ?", ownerID, root.Id).Count(&holders).Error; err != nil {
			return err
		}
		if holders > 0 {
			return errWtOwnerDiscordIDTaken
		}
		if root.Id != 0 {
			common.SysLog(common.LogText("binding the root account to WT_OWNER_DISCORD_ID and revoking its credentials"))
			if _, err := IncrementUserAuthVersionWithTx(tx, root.Id); err != nil {
				return err
			}
			var err error
			if revoked, err = revokeWtAccessWithTx(tx, root.Id); err != nil {
				return err
			}
			if err := tx.Model(&PasskeyCredential{}).Where("user_id = ?", root.Id).Count(&revoked.Passkeys).Error; err != nil {
				return err
			}
			if err := tx.Model(&TwoFA{}).Where("user_id = ? AND is_enabled = ?", root.Id, true).Count(&revoked.TwoFactor).Error; err != nil {
				return err
			}
			for _, factor := range []any{&PasskeyCredential{}, &TwoFABackupCode{}, &TwoFA{}} {
				if err := tx.Unscoped().Where("user_id = ?", root.Id).Delete(factor).Error; err != nil {
					return err
				}
			}
			rebound = root.Id
			return tx.Model(&User{}).Where("id = ?", root.Id).Updates(map[string]any{"discord_id": ownerID, "password": ""}).Error
		}
		username := "root"
		var taken int64
		if err := tx.Unscoped().Model(&User{}).Where("username = ?", username).Count(&taken).Error; err != nil {
			return err
		}
		if taken > 0 {
			username = "wt_owner"
		}
		owner := User{
			Username:    username,
			DisplayName: "Root User",
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DiscordId:   ownerID,
			Quota:       wtOwnerInitialQuota,
			AffCode:     common.GetRandomString(4),
		}
		owner.SetSetting(dto.UserSetting{})
		common.SysLog(common.LogText("creating the root account for WT_OWNER_DISCORD_ID"))
		return tx.Create(&owner).Error
	})
	if err != nil || rebound == 0 {
		return err
	}
	RecordOperationAuditLog(rebound, common.RoleRootUser, "Bound the root account to a new owner Discord account and revoked its credentials", "", "wt.owner_rebound",
		map[string]any{"access_tokens": revoked.AccessTokens, "relay_keys": revoked.RelayKeys, "passkeys": revoked.Passkeys, "two_factor": revoked.TwoFactor}, nil, nil)
	// Same follow-up as upstream user management after an auth change:
	// publish the new auth version, revoke browser sessions and drop cached
	// API tokens.
	if err := PublishUserAuthCache(rebound); err != nil {
		common.SysError(common.LogText("failed to publish the auth cache of user %d: %s", rebound, err.Error()))
	}
	if _, err := RevokeAllUserSessions(rebound, "wt_owner_rebound"); err != nil {
		return err
	}
	if err := InvalidateUserTokensCache(rebound); err != nil {
		common.SysError(common.LogText("failed to invalidate tokens cache for user %d: %s", rebound, err.Error()))
	}
	return nil
}

// WtOwnerAccount returns the owner's root account, the root bound to
// WT_OWNER_DISCORD_ID.
func WtOwnerAccount() (*User, error) {
	ownerID := WtOwnerDiscordID()
	if ownerID == "" {
		return nil, ErrWtOwnerNotReady
	}
	var owner User
	if err := DB.Where("role = ? AND discord_id = ?", common.RoleRootUser, ownerID).Order("id").First(&owner).Error; err != nil {
		return nil, wtNotFound(err, ErrWtOwnerNotReady)
	}
	return &owner, nil
}

func wtEmergencyTokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

// IssueWtEmergencyToken creates a single-use emergency login token that is
// valid for WtEmergencyTokenTTL. Every older token stops working. Only the
// digest is stored; the caller shows the token once and must not log it.
func IssueWtEmergencyToken(now time.Time) (string, error) {
	raw := make([]byte, wtEmergencyTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	row := WtEmergencyToken{
		TokenHash: wtEmergencyTokenHash(token),
		ExpiresAt: now.Add(WtEmergencyTokenTTL).Unix(),
		CreatedAt: now.Unix(),
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id > ?", 0).Delete(&WtEmergencyToken{}).Error; err != nil {
			return err
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// ConsumeWtEmergencyToken accepts a token once, before it expires and only if
// no newer token replaced it. Every failure is ErrWtEmergencyTokenInvalid or a
// database error, so callers can answer all of them with one message.
func ConsumeWtEmergencyToken(token string, now time.Time) error {
	token = strings.TrimSpace(token)
	if len(token) != base64.RawURLEncoding.EncodedLen(wtEmergencyTokenBytes) {
		return ErrWtEmergencyTokenInvalid
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var row WtEmergencyToken
		err := lockForUpdate(tx.Where("token_hash = ?", wtEmergencyTokenHash(token))).First(&row).Error
		if err != nil {
			return wtNotFound(err, ErrWtEmergencyTokenInvalid)
		}
		if row.UsedAt != 0 || now.Unix() >= row.ExpiresAt {
			return ErrWtEmergencyTokenInvalid
		}
		result := tx.Model(&WtEmergencyToken{}).Where("id = ? AND used_at = ?", row.Id, 0).Update("used_at", now.Unix())
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrWtEmergencyTokenInvalid
		}
		return nil
	})
}

// WtOpenDatabase connects to the main database, and to the log database when
// LOG_SQL_DSN names one, for the `wt` maintenance commands without running
// migrations, so it is safe next to a running server.
func WtOpenDatabase() error {
	db, dbType, err := chooseDB("SQL_DSN", false)
	if err != nil {
		return err
	}
	common.SetMainDatabaseType(dbType)
	common.SetLogDatabaseType(dbType)
	DB, LOG_DB = db, db
	if os.Getenv("LOG_SQL_DSN") != "" {
		logDB, logType, err := chooseDB("LOG_SQL_DSN", true)
		if err != nil {
			return err
		}
		common.SetLogDatabaseType(logType)
		LOG_DB = logDB
	}
	initCol()
	if !DB.Migrator().HasTable(&WtEmergencyToken{}) {
		return fmt.Errorf("table %s is missing; start the server once so it migrates the database", WtEmergencyToken{}.TableName())
	}
	return nil
}
