package model

import (
	"crypto/hmac"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// wind-tide: Discord allowlist, entry requests and the Discord sign-in gate
// (wind-tide/API.md §3 and §5).

const (
	WtAllowlistActive  = "active"
	WtAllowlistRemoved = "removed"

	WtEntryPending  = "pending"
	WtEntryApproved = "approved"
	WtEntryRejected = "rejected"

	// WtGateRemoved is the gate answer for a Discord account removed from the
	// allowlist. The other answers reuse the entry request states.
	WtGateRemoved = "removed"

	// The states of the account an allowlist entry admits, as the owner sees
	// them; an entry whose Discord account has not signed in yet has none.
	WtAccountEnabled  = "enabled"
	WtAccountDisabled = "disabled"
	WtAccountDeleted  = "deleted"

	// WtEntryRetryDelay is how long a rejected applicant waits before the next
	// sign-in files a new entry request.
	WtEntryRetryDelay = 24 * time.Hour

	wtNameMaxLength = 64
	wtNoteMaxLength = 255
)

var (
	wtDiscordIDPattern  = regexp.MustCompile(`^[0-9]{1,20}$`)
	wtAvatarHashPattern = regexp.MustCompile(`^(a_)?[0-9a-f]{32}$`)
)

var (
	ErrWtDiscordIDInvalid     = common.NewMessage("Invalid Discord user ID")
	ErrWtAllowlistIdentity    = common.NewMessage("Enter a Discord user ID")
	ErrWtAllowlistDuplicate   = common.NewMessage("This Discord account is already on the allowlist")
	ErrWtAllowlistNotFound    = common.NewMessage("Allowlist entry does not exist")
	ErrWtAllowlistOwner       = common.NewMessage("The owner does not need an allowlist entry")
	ErrWtAllowlistChanged     = common.NewMessage("This allowlist entry has changed, refresh and try again")
	ErrWtAllowlistNotRemoved  = common.NewMessage("Only a removed allowlist entry can be restored")
	ErrWtOwnerRemoval         = common.NewMessage("The owner cannot be removed from the allowlist")
	ErrWtEntryRequestNotFound = common.NewMessage("Entry request does not exist")
	ErrWtEntryRequestDecided  = common.NewMessage("This entry request has already been handled")
	ErrWtApplicantRemoved     = common.NewMessage("This Discord account has been removed, restore it from the allowlist instead")
	ErrWtAccountNotListed     = common.NewMessage("Only accounts on the allowlist can be enabled")
	ErrWtDiscordOnly          = common.NewMessage("This site only supports signing in with Discord")
	errWtInvalidParameters    = common.NewMessage("Invalid parameters")

	// errWtGateRetry asks the sign-in gate to look at the allowlist again: an
	// entry for the Discord account appeared while the gate was filing its
	// entry request.
	errWtGateRetry = errors.New("the allowlist changed during sign-in")
)

// WtAllowlistEntry admits one Discord account. DiscordId stays NULL while a
// username pre-fill, added before the allowlist took Discord user IDs only,
// waits for its name to sign in, and UsernameKey is the lower-cased username
// of such a pre-fill. Both columns are nullable so that their unique indexes
// ignore missing values on SQLite, MySQL and PostgreSQL alike.
type WtAllowlistEntry struct {
	Id              int     `json:"id"`
	DiscordId       *string `json:"discord_id" gorm:"type:varchar(32);uniqueIndex:idx_wt_allowlist_discord_id"`
	DiscordUsername string  `json:"discord_username" gorm:"type:varchar(64);not null"`
	UsernameKey     *string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_wt_allowlist_username_key"`
	Note            string  `json:"note" gorm:"type:varchar(255);not null"`
	Status          string  `json:"status" gorm:"type:varchar(16);not null;index:idx_wt_allowlist_status"`
	UserId          int     `json:"user_id" gorm:"not null;index:idx_wt_allowlist_user_id"`
	CreatedAt       int64   `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt       int64   `json:"updated_at" gorm:"type:bigint;not null"`
}

func (WtAllowlistEntry) TableName() string {
	return "wt_allowlist"
}

// WtEntryRequest is the single entry request of one Discord account.
type WtEntryRequest struct {
	Id              int    `json:"id"`
	DiscordId       string `json:"discord_id" gorm:"type:varchar(32);not null;uniqueIndex:idx_wt_entry_requests_discord_id"`
	DiscordUsername string `json:"discord_username" gorm:"type:varchar(64);not null"`
	DisplayName     string `json:"display_name" gorm:"type:varchar(64);not null"`
	Status          string `json:"status" gorm:"type:varchar(16);not null;index:idx_wt_entry_requests_status"`
	RequestedAt     int64  `json:"requested_at" gorm:"type:bigint;not null"`
	DecidedAt       int64  `json:"decided_at" gorm:"type:bigint;not null"`
}

func (WtEntryRequest) TableName() string {
	return "wt_entry_requests"
}

// WtDiscordProfile keeps what Discord last reported for an account, so names
// and cached avatars can be shown for the owner, members and applicants.
type WtDiscordProfile struct {
	DiscordId  string `gorm:"primaryKey;type:varchar(32)"`
	Username   string `gorm:"type:varchar(64);not null"`
	GlobalName string `gorm:"type:varchar(64);not null"`
	AvatarHash string `gorm:"type:varchar(40);not null"`
	UpdatedAt  int64  `gorm:"type:bigint;not null"`
}

func (WtDiscordProfile) TableName() string {
	return "wt_discord_profiles"
}

// wtModels are the tables owned by the wind-tide fork; migrateDB migrates them
// together with the upstream tables.
var wtModels = []any{
	&WtAllowlistEntry{},
	&WtEntryRequest{},
	&WtDiscordProfile{},
	&WtEmergencyToken{},
}

// WtValidDiscordID reports whether id has the shape of a Discord user ID.
func WtValidDiscordID(id string) bool {
	return wtDiscordIDPattern.MatchString(id)
}

// WtValidAvatarHash reports whether hash has the shape of a Discord avatar hash.
func WtValidAvatarHash(hash string) bool {
	return wtAvatarHashPattern.MatchString(hash)
}

// wtAvatarSignature signs an avatar address of a Discord ID that expires at
// expiresAt, with a key derived from the session secret.
func wtAvatarSignature(discordID string, expiresAt int64) string {
	key := common.GenerateHMACWithKey([]byte(common.SessionSecret), "wind-tide avatar address")
	return common.GenerateHMACWithKey([]byte(key), discordID+"."+strconv.FormatInt(expiresAt, 10))
}

// WtAvatarURL is the signed site address of a cached Discord avatar, or ""
// when the account has no custom avatar. Only the people the site showed the
// address to can fetch the avatar, so the avatar endpoint does not tell
// strangers which Discord IDs belong to the site. The address stays the same
// for a whole Beijing day and expires at the end of the next one.
func WtAvatarURL(discordID, hash string) string {
	if !WtValidDiscordID(discordID) || !WtValidAvatarHash(hash) {
		return ""
	}
	expiresAt := wtCheckinDayStart(WtNow()).AddDate(0, 0, 2).Unix()
	return "/api/wt/avatar/" + discordID + "?v=" + hash +
		"&exp=" + strconv.FormatInt(expiresAt, 10) + "&sig=" + wtAvatarSignature(discordID, expiresAt)
}

// WtAvatarURLTTL checks the exp and sig of an avatar address at now and
// returns how many seconds the address stays valid.
func WtAvatarURLTTL(discordID, exp, sig string, now time.Time) (int64, bool) {
	expiresAt, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || !WtValidDiscordID(discordID) {
		return 0, false
	}
	if !hmac.Equal([]byte(sig), []byte(wtAvatarSignature(discordID, expiresAt))) || expiresAt <= now.Unix() {
		return 0, false
	}
	return expiresAt - now.Unix(), true
}

// WtAvatarURLForDiscordID looks up the cached avatar address of a Discord
// account. Lookup failures fall back to the default avatar.
func WtAvatarURLForDiscordID(discordID string) string {
	if !WtValidDiscordID(discordID) {
		return ""
	}
	var profile WtDiscordProfile
	if err := DB.Where("discord_id = ?", discordID).Limit(1).Find(&profile).Error; err != nil {
		return ""
	}
	return WtAvatarURL(discordID, profile.AvatarHash)
}

// WtKnownAvatarHash returns the avatar hash of a Discord ID that belongs to an
// account or an entry request. Other IDs are unknown, so the avatar endpoint
// never fetches or serves avatars for arbitrary IDs.
func WtKnownAvatarHash(discordID string) (string, bool, error) {
	if !WtValidDiscordID(discordID) {
		return "", false, nil
	}
	var accounts, requests int64
	if err := DB.Model(&User{}).Where("discord_id = ?", discordID).Count(&accounts).Error; err != nil {
		return "", false, err
	}
	if accounts == 0 {
		if err := DB.Model(&WtEntryRequest{}).Where("discord_id = ?", discordID).Count(&requests).Error; err != nil {
			return "", false, err
		}
		if requests == 0 {
			return "", false, nil
		}
	}
	var profile WtDiscordProfile
	if err := DB.Where("discord_id = ?", discordID).Limit(1).Find(&profile).Error; err != nil {
		return "", false, err
	}
	return profile.AvatarHash, true, nil
}

// wtClip trims value and keeps at most limit characters, matching the column
// sizes of the wind-tide tables.
func wtClip(value string, limit int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

// wtUsernameKey is the case-insensitive form of a Discord username used to
// match username pre-fills.
func wtUsernameKey(username string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
}

func wtNotFound(err error, notFound error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound
	}
	return err
}

// findWtAllowlistEntryWithTx locks the allowlist entry of a Discord ID and
// returns nil when there is none. The allowlist is always locked before the
// entry requests and the accounts.
func findWtAllowlistEntryWithTx(tx *gorm.DB, discordID string) (*WtAllowlistEntry, error) {
	var entry WtAllowlistEntry
	if err := lockForUpdate(tx.Where("discord_id = ?", discordID)).Limit(1).Find(&entry).Error; err != nil {
		return nil, err
	}
	if entry.Id == 0 {
		return nil, nil
	}
	return &entry, nil
}

// refuseWtUnlistedAccountWithTx refuses to enable an account that no active
// allowlist entry admits, by the linked account or by its Discord ID. Only
// root needs no entry. A removed member comes back only by restoring the
// entry, with the owner's step-up verification, which enables the account
// together with it.
func refuseWtUnlistedAccountWithTx(tx *gorm.DB, user *User) error {
	if user.Role == common.RoleRootUser {
		return nil
	}
	var entries []WtAllowlistEntry
	if err := lockForUpdate(tx.Where("user_id = ? OR discord_id = ?", user.Id, user.DiscordId)).Find(&entries).Error; err != nil {
		return err
	}
	if len(entries) == 0 {
		return ErrWtAccountNotListed
	}
	for _, entry := range entries {
		if entry.Status == WtAllowlistActive {
			return nil
		}
	}
	return ErrWtApplicantRemoved
}

// WtDiscordIdentity is the Discord account presented at sign-in.
type WtDiscordIdentity struct {
	DiscordId  string
	Username   string
	GlobalName string
	AvatarHash string
}

// DisplayName is the name the site shows for the account: the Discord global
// name, or the username when there is none.
func (identity WtDiscordIdentity) DisplayName() string {
	if identity.GlobalName != "" {
		return identity.GlobalName
	}
	return identity.Username
}

// WtGateDecision is the answer of the sign-in gate. User is set when the
// account is admitted; otherwise Gate holds pending, rejected or removed.
// PinnedEntryId is the username pre-fill this sign-in pinned to the Discord
// ID, for the audit log.
type WtGateDecision struct {
	User          *User
	Gate          string
	RequestedAt   int64
	RetryAfter    int64
	PinnedEntryId int
}

// WtAdmitDiscordAccount runs the sign-in gate for a Discord account in the
// order of wind-tide/API.md §3: the owner, an active allowlist entry by ID, an
// active username pre-fill that is pinned to the ID on first sign-in, a
// removed entry, and otherwise the single entry request of the Discord ID.
//
// Names are stored best-effort, outside the gate's transaction: a MySQL
// database in utf8mb3 cannot store every name Discord allows (emoji), and that
// must not keep anyone out.
func WtAdmitDiscordAccount(identity WtDiscordIdentity, now time.Time) (*WtGateDecision, error) {
	identity.DiscordId = strings.TrimSpace(identity.DiscordId)
	if !WtValidDiscordID(identity.DiscordId) {
		return nil, ErrWtDiscordIDInvalid
	}
	identity.Username = wtClip(identity.Username, wtNameMaxLength)
	identity.GlobalName = wtClip(identity.GlobalName, wtNameMaxLength)
	if !WtValidAvatarHash(identity.AvatarHash) {
		identity.AvatarHash = ""
	}
	profile := WtDiscordProfile{
		DiscordId:  identity.DiscordId,
		Username:   identity.Username,
		GlobalName: identity.GlobalName,
		AvatarHash: identity.AvatarHash,
		UpdatedAt:  now.Unix(),
	}
	if err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "discord_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"username", "global_name", "avatar_hash", "updated_at"}),
	}).Create(&profile).Error; err != nil {
		common.SysError(common.LogText("failed to save the Discord profile of %s: %s", identity.DiscordId, err.Error()))
	}

	if identity.DiscordId == WtOwnerDiscordID() {
		owner, err := WtOwnerAccount()
		if err != nil {
			return nil, err
		}
		syncWtDisplayName(owner, identity.DisplayName())
		return &WtGateDecision{User: owner}, nil
	}

	var decision *WtGateDecision
	created := false
	var err error
	for range 2 {
		decision, created = &WtGateDecision{}, false
		err = DB.Transaction(func(tx *gorm.DB) error {
			entry, err := findWtAllowlistEntryWithTx(tx, identity.DiscordId)
			if err != nil {
				return err
			}
			if entry == nil {
				if entry, err = findWtPrefillWithTx(tx, identity.Username); err != nil {
					return err
				}
				pinned := false
				if entry != nil {
					if pinned, err = pinWtPrefillWithTx(tx, entry, identity.DiscordId, identity.Username, now); err != nil {
						return err
					}
				}
				if !pinned {
					return refreshWtEntryRequestWithTx(tx, identity, now, decision)
				}
				decision.PinnedEntryId = entry.Id
			}
			if entry.Status != WtAllowlistActive {
				decision.Gate = WtGateRemoved
				return nil
			}
			decision.User, created, err = ensureWtMemberWithTx(tx, entry, identity, now)
			if err == nil && decision.User == nil {
				decision.Gate = WtGateRemoved
			}
			return err
		})
		if !errors.Is(err, errWtGateRetry) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	if decision.User == nil {
		if decision.Gate != WtGateRemoved {
			if err := DB.Model(&WtEntryRequest{}).Where("discord_id = ?", identity.DiscordId).
				Update("display_name", identity.DisplayName()).Error; err != nil {
				common.SysError(common.LogText("failed to store the name of the entry request of %s: %s", identity.DiscordId, err.Error()))
			}
		}
		return decision, nil
	}
	if created {
		decision.User.FinalizeOAuthUserCreation(0)
	}
	syncWtDisplayName(decision.User, identity.DisplayName())
	return decision, nil
}

// syncWtDisplayName keeps an account's display name equal to its latest
// Discord name, best-effort.
func syncWtDisplayName(user *User, name string) {
	if name == "" || user.DisplayName == name {
		return
	}
	if err := DB.Model(&User{}).Where("id = ?", user.Id).Update("display_name", name).Error; err != nil {
		common.SysError(common.LogText("failed to store the display name of user %d: %s", user.Id, err.Error()))
		return
	}
	user.DisplayName = name
}

// findWtPrefillWithTx locks the oldest active username pre-fill that names
// username and has no Discord ID yet, and returns nil when there is none.
// Pre-fills were added before the allowlist took Discord user IDs only; each
// is pinned to the applicant the owner approves under its name, or else to
// the first account that signs in with the name.
func findWtPrefillWithTx(tx *gorm.DB, username string) (*WtAllowlistEntry, error) {
	key := wtUsernameKey(username)
	if key == "" {
		return nil, nil
	}
	var entry WtAllowlistEntry
	if err := lockForUpdate(tx.Where("username_key = ? AND discord_id IS NULL AND status = ?", key, WtAllowlistActive)).
		Order("id").Limit(1).Find(&entry).Error; err != nil {
		return nil, err
	}
	if entry.Id == 0 {
		return nil, nil
	}
	return &entry, nil
}

// pinWtPrefillWithTx binds a username pre-fill to the Discord account that
// owns the name, links the account that already holds the Discord ID if there
// is one, and approves the pending entry request of the ID. From then on the
// pre-fill admits that Discord account and no other one. It returns false
// when the entry was pinned by someone else first.
func pinWtPrefillWithTx(tx *gorm.DB, entry *WtAllowlistEntry, discordID, username string, now time.Time) (bool, error) {
	var holder User
	if err := tx.Select("id").Where("discord_id = ?", discordID).Order("id").Limit(1).Find(&holder).Error; err != nil {
		return false, err
	}
	result := tx.Model(&WtAllowlistEntry{}).Where("id = ? AND discord_id IS NULL", entry.Id).Updates(map[string]any{
		"discord_id":       discordID,
		"discord_username": username,
		"user_id":          holder.Id,
		"updated_at":       now.Unix(),
	})
	if result.Error != nil || result.RowsAffected != 1 {
		return false, result.Error
	}
	if err := tx.Model(&WtEntryRequest{}).Where("discord_id = ? AND status = ?", discordID, WtEntryPending).
		Updates(map[string]any{"status": WtEntryApproved, "decided_at": now.Unix()}).Error; err != nil {
		return false, err
	}
	entry.DiscordId, entry.DiscordUsername, entry.UserId, entry.UpdatedAt = &discordID, username, holder.Id, now.Unix()
	return true, nil
}

// ensureWtMemberWithTx returns the account of an active allowlist entry. On the
// first sign-in it creates the member account the way upstream OAuth
// registration does (role 1, the new-user quota) and links the entry to it.
// When the linked account was deleted, the entry is marked removed instead
// and nil is returned: the owner decides whether the Discord account may come
// back, with a new account, by restoring the entry. A root account is never
// admitted here; only the owner signs in to root.
func ensureWtMemberWithTx(tx *gorm.DB, entry *WtAllowlistEntry, identity WtDiscordIdentity, now time.Time) (*User, bool, error) {
	var user User
	if err := tx.Where("discord_id = ?", identity.DiscordId).Order("id").Limit(1).Find(&user).Error; err != nil {
		return nil, false, err
	}
	rebind := false
	if user.Id == 0 && entry.UserId != 0 {
		if err := tx.Where("id = ?", entry.UserId).Limit(1).Find(&user).Error; err != nil {
			return nil, false, err
		}
		if user.Id == 0 || user.DiscordId != "" {
			entry.Status, entry.UpdatedAt = WtAllowlistRemoved, now.Unix()
			return nil, false, tx.Model(&WtAllowlistEntry{}).Where("id = ?", entry.Id).Updates(map[string]any{
				"status":     entry.Status,
				"updated_at": entry.UpdatedAt,
			}).Error
		}
		// The account lost its Discord binding before wind-tide stopped
		// clearing it; the entry still says it belongs to this Discord ID.
		rebind = true
	}
	if user.Role == common.RoleRootUser {
		return nil, false, ErrWtOwnerNotReady
	}
	if rebind {
		if err := tx.Model(&User{}).Where("id = ?", user.Id).Update("discord_id", identity.DiscordId).Error; err != nil {
			return nil, false, err
		}
	}
	created := false
	if user.Id == 0 {
		// An account deleted before the owner admitted this Discord ID keeps
		// the ID; release it so the new account can take it.
		if err := tx.Unscoped().Model(&User{}).Where("discord_id = ? AND deleted_at IS NOT NULL", identity.DiscordId).
			Update("discord_id", "").Error; err != nil {
			return nil, false, err
		}
		username := ""
		if candidate := identity.Username; candidate != "" && len(candidate) <= UserNameMaxLength {
			var taken int64
			if err := tx.Unscoped().Model(&User{}).Where("username = ?", candidate).Count(&taken).Error; err != nil {
				return nil, false, err
			}
			if taken == 0 {
				username = candidate
			}
		}
		if username == "" {
			var last User
			if err := tx.Unscoped().Order("id DESC").Limit(1).Find(&last).Error; err != nil {
				return nil, false, err
			}
			for next := last.Id + 1; username == ""; next++ {
				candidate := "discord_" + strconv.Itoa(next)
				var taken int64
				if err := tx.Unscoped().Model(&User{}).Where("username = ?", candidate).Count(&taken).Error; err != nil {
					return nil, false, err
				}
				if taken == 0 {
					username = candidate
				}
			}
		}
		// The Discord username is a safe first display name; the Discord name
		// is stored after the transaction (see WtAdmitDiscordAccount).
		user = User{
			Username:    username,
			DisplayName: identity.Username,
			DiscordId:   identity.DiscordId,
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
		}
		if err := user.InsertWithTx(tx, 0); err != nil {
			return nil, false, err
		}
		created = true
	}
	if entry.UserId != user.Id || entry.DiscordUsername != identity.Username {
		if err := tx.Model(&WtAllowlistEntry{}).Where("id = ?", entry.Id).Updates(map[string]any{
			"user_id":          user.Id,
			"discord_username": identity.Username,
			"updated_at":       now.Unix(),
		}).Error; err != nil {
			return nil, false, err
		}
		entry.UserId, entry.DiscordUsername, entry.UpdatedAt = user.Id, identity.Username, now.Unix()
	}
	if err := tx.First(&user, user.Id).Error; err != nil {
		return nil, false, err
	}
	return &user, created, nil
}

// refreshWtEntryRequestWithTx files or refreshes the single entry request of a
// Discord account outside the allowlist and records the gate answer: rejected
// within WtEntryRetryDelay of the decision, pending otherwise. A request the
// owner approved while this sign-in was running is left alone and the gate
// looks at the allowlist again.
func refreshWtEntryRequestWithTx(tx *gorm.DB, identity WtDiscordIdentity, now time.Time, decision *WtGateDecision) error {
	filed := WtEntryRequest{
		DiscordId:       identity.DiscordId,
		DiscordUsername: identity.Username,
		DisplayName:     identity.Username,
		Status:          WtEntryPending,
		RequestedAt:     now.Unix(),
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&filed).Error; err != nil {
		return err
	}
	var request WtEntryRequest
	if err := lockForUpdate(tx.Where("discord_id = ?", identity.DiscordId)).First(&request).Error; err != nil {
		return err
	}
	if request.Status == WtEntryApproved {
		entry, err := findWtAllowlistEntryWithTx(tx, identity.DiscordId)
		if err != nil {
			return err
		}
		if entry != nil {
			return errWtGateRetry
		}
	}
	updates := map[string]any{"discord_username": identity.Username}
	retryAt := request.DecidedAt + int64(WtEntryRetryDelay/time.Second)
	switch {
	case request.Status == WtEntryRejected && now.Unix() < retryAt:
		decision.Gate, decision.RetryAfter = WtEntryRejected, retryAt
	case request.Status == WtEntryPending:
		decision.Gate = WtEntryPending
	default:
		updates["status"], updates["requested_at"], updates["decided_at"] = WtEntryPending, now.Unix(), int64(0)
		request.RequestedAt = now.Unix()
		decision.Gate = WtEntryPending
	}
	decision.RequestedAt = request.RequestedAt
	return tx.Model(&WtEntryRequest{}).Where("id = ?", request.Id).Updates(updates).Error
}

// WtAllowlistInput is what the owner enters for a new allowlist entry.
type WtAllowlistInput struct {
	DiscordId string `json:"discord_id"`
	Note      string `json:"note"`
}

// WtAllowlistItem is an allowlist entry as the owner sees it. UserId is the
// account the entry admits, 0 before the Discord account signs in or after
// the linked account was deleted; AccountStatus is that account's state.
type WtAllowlistItem struct {
	Id              int    `json:"id"`
	DiscordId       string `json:"discord_id"`
	DiscordUsername string `json:"discord_username"`
	Note            string `json:"note"`
	Status          string `json:"status"`
	UserId          int    `json:"user_id"`
	AccountStatus   string `json:"account_status"`
	DisplayName     string `json:"display_name"`
	AvatarURL       string `json:"avatar_url"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

// WtEntryRequestItem is an entry request as the owner sees it.
type WtEntryRequestItem struct {
	Id              int    `json:"id"`
	DiscordId       string `json:"discord_id"`
	DiscordUsername string `json:"discord_username"`
	DisplayName     string `json:"display_name"`
	AvatarURL       string `json:"avatar_url"`
	Status          string `json:"status"`
	RequestedAt     int64  `json:"requested_at"`
	DecidedAt       int64  `json:"decided_at"`
}

// wtProfiles loads the Discord profiles of the given IDs.
func wtProfiles(discordIDs []string) (map[string]WtDiscordProfile, error) {
	profiles := make(map[string]WtDiscordProfile, len(discordIDs))
	if len(discordIDs) == 0 {
		return profiles, nil
	}
	var rows []WtDiscordProfile
	if err := DB.Where("discord_id IN ?", discordIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		profiles[row.DiscordId] = row
	}
	return profiles, nil
}

// wtAllowlistAccount is the live account an allowlist entry admits: the
// linked account, or before the first sign-in an account that already holds
// the Discord ID. It is nil when there is none or the linked account was
// deleted. wtAllowlistItems resolves accounts the same way for the list.
func wtAllowlistAccount(query *gorm.DB, entry *WtAllowlistEntry) (*User, error) {
	switch {
	case entry.UserId != 0:
		query = query.Where("id = ?", entry.UserId)
	case entry.DiscordId != nil:
		query = query.Where("discord_id = ?", *entry.DiscordId).Order("id")
	default:
		return nil, nil
	}
	var user User
	if err := query.Limit(1).Find(&user).Error; err != nil {
		return nil, err
	}
	if user.Id == 0 {
		return nil, nil
	}
	return &user, nil
}

// wtAllowlistItems adds the account, its state, the name and the avatar to
// allowlist entries.
func wtAllowlistItems(entries []WtAllowlistEntry) ([]WtAllowlistItem, error) {
	userIDs := make([]int, 0, len(entries))
	discordIDs := make([]string, 0, len(entries))
	unlinked := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.UserId != 0 {
			userIDs = append(userIDs, entry.UserId)
		}
		if entry.DiscordId != nil {
			discordIDs = append(discordIDs, *entry.DiscordId)
			if entry.UserId == 0 {
				unlinked = append(unlinked, *entry.DiscordId)
			}
		}
	}
	linked := make(map[int]User, len(userIDs))
	if len(userIDs) > 0 {
		var users []User
		if err := DB.Unscoped().Select("id", "username", "display_name", "status", "deleted_at").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, err
		}
		for _, user := range users {
			linked[user.Id] = user
		}
	}
	holders := make(map[string]User, len(unlinked))
	if len(unlinked) > 0 {
		var users []User
		if err := DB.Select("id", "username", "display_name", "status", "discord_id").Where("discord_id IN ?", unlinked).Order("id DESC").Find(&users).Error; err != nil {
			return nil, err
		}
		for _, user := range users {
			holders[user.DiscordId] = user // the lowest id comes last and wins, like in wtAllowlistAccount
		}
	}
	profiles, err := wtProfiles(discordIDs)
	if err != nil {
		return nil, err
	}
	items := make([]WtAllowlistItem, 0, len(entries))
	for _, entry := range entries {
		item := WtAllowlistItem{
			Id:              entry.Id,
			DiscordUsername: entry.DiscordUsername,
			Note:            entry.Note,
			Status:          entry.Status,
			CreatedAt:       entry.CreatedAt,
			UpdatedAt:       entry.UpdatedAt,
		}
		account, found := linked[entry.UserId]
		if entry.UserId == 0 && entry.DiscordId != nil {
			account, found = holders[*entry.DiscordId]
		}
		switch {
		case entry.UserId != 0 && (!found || account.DeletedAt.Valid):
			item.AccountStatus = WtAccountDeleted
		case found:
			item.UserId, item.AccountStatus = account.Id, WtAccountDisabled
			if account.Status == common.UserStatusEnabled {
				item.AccountStatus = WtAccountEnabled
			}
			item.DisplayName = account.DisplayName
			if item.DisplayName == "" {
				item.DisplayName = account.Username
			}
		}
		if entry.DiscordId != nil {
			item.DiscordId = *entry.DiscordId
			profile := profiles[item.DiscordId]
			item.AvatarURL = WtAvatarURL(item.DiscordId, profile.AvatarHash)
			if item.DisplayName == "" {
				item.DisplayName = WtDiscordIdentity{Username: profile.Username, GlobalName: profile.GlobalName}.DisplayName()
			}
		}
		items = append(items, item)
	}
	return items, nil
}

// wtEntryRequestItems adds the cached avatar to entry requests.
func wtEntryRequestItems(requests []WtEntryRequest) ([]WtEntryRequestItem, error) {
	discordIDs := make([]string, 0, len(requests))
	for _, request := range requests {
		discordIDs = append(discordIDs, request.DiscordId)
	}
	profiles, err := wtProfiles(discordIDs)
	if err != nil {
		return nil, err
	}
	items := make([]WtEntryRequestItem, 0, len(requests))
	for _, request := range requests {
		items = append(items, WtEntryRequestItem{
			Id:              request.Id,
			DiscordId:       request.DiscordId,
			DiscordUsername: request.DiscordUsername,
			DisplayName:     request.DisplayName,
			AvatarURL:       WtAvatarURL(request.DiscordId, profiles[request.DiscordId].AvatarHash),
			Status:          request.Status,
			RequestedAt:     request.RequestedAt,
			DecidedAt:       request.DecidedAt,
		})
	}
	return items, nil
}

func wtAllowlistItem(entry WtAllowlistEntry) (*WtAllowlistItem, error) {
	items, err := wtAllowlistItems([]WtAllowlistEntry{entry})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

// WtListAllowlist lists allowlist entries, newest first. keyword matches the
// Discord ID, username or note; status is active, removed or empty for all.
func WtListAllowlist(keyword, status string, startIdx, num int) ([]WtAllowlistItem, int64, error) {
	query := DB.Model(&WtAllowlistEntry{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if keyword = strings.ToLower(strings.TrimSpace(keyword)); keyword != "" {
		pattern := "%" + keyword + "%"
		query = query.Where("(discord_id LIKE ? OR LOWER(discord_username) LIKE ? OR LOWER(note) LIKE ?)", pattern, pattern, pattern)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var entries []WtAllowlistEntry
	if err := query.Order("id DESC").Limit(num).Offset(startIdx).Find(&entries).Error; err != nil {
		return nil, 0, err
	}
	items, err := wtAllowlistItems(entries)
	return items, total, err
}

// wtAllowlistDuplicate reports whether an entry already names the Discord ID.
func wtAllowlistDuplicate(db *gorm.DB, discordID string) (bool, error) {
	var count int64
	err := db.Model(&WtAllowlistEntry{}).Where("discord_id = ?", discordID).Count(&count).Error
	return count > 0, err
}

// WtAddAllowlistEntry adds an active entry for a Discord user ID, and the
// pending entry request of the ID is approved with it. Entries name an ID
// only: anyone can take a Discord username nobody holds, so a name identifies
// nobody, and a member known only by name signs in once to file an entry
// request instead. The username pre-fills added before keep working, see
// findWtPrefillWithTx. The owner signs in without an entry, so the owner's ID
// is refused.
func WtAddAllowlistEntry(input WtAllowlistInput, now time.Time) (*WtAllowlistItem, error) {
	discordID := strings.TrimSpace(input.DiscordId)
	note := strings.TrimSpace(input.Note)
	if discordID == "" {
		return nil, ErrWtAllowlistIdentity
	}
	if !WtValidDiscordID(discordID) {
		return nil, ErrWtDiscordIDInvalid
	}
	if discordID == WtOwnerDiscordID() {
		return nil, ErrWtAllowlistOwner
	}
	if utf8.RuneCountInString(note) > wtNoteMaxLength {
		return nil, errWtInvalidParameters
	}
	entry := WtAllowlistEntry{
		DiscordId: &discordID,
		Note:      note,
		Status:    WtAllowlistActive,
		CreatedAt: now.Unix(),
		UpdatedAt: now.Unix(),
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		duplicate, err := wtAllowlistDuplicate(tx, discordID)
		if err != nil {
			return err
		}
		if duplicate {
			return ErrWtAllowlistDuplicate
		}
		var user User
		if err := tx.Select("id").Where("discord_id = ?", discordID).Order("id").Limit(1).Find(&user).Error; err != nil {
			return err
		}
		entry.UserId = user.Id
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		return tx.Model(&WtEntryRequest{}).Where("discord_id = ? AND status = ?", discordID, WtEntryPending).
			Updates(map[string]any{"status": WtEntryApproved, "decided_at": now.Unix()}).Error
	})
	if err != nil && !errors.Is(err, ErrWtAllowlistDuplicate) {
		// A concurrent add of the same account fails on the unique index;
		// answer it like any other duplicate.
		if duplicate, checkErr := wtAllowlistDuplicate(DB, discordID); checkErr == nil && duplicate {
			return nil, ErrWtAllowlistDuplicate
		}
	}
	if err != nil {
		return nil, err
	}
	return wtAllowlistItem(entry)
}

// WtUpdateAllowlistNote replaces the note of an allowlist entry.
func WtUpdateAllowlistNote(id int, note string, now time.Time) (*WtAllowlistItem, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > wtNoteMaxLength {
		return nil, errWtInvalidParameters
	}
	var entry WtAllowlistEntry
	if err := DB.First(&entry, id).Error; err != nil {
		return nil, wtNotFound(err, ErrWtAllowlistNotFound)
	}
	if err := DB.Model(&WtAllowlistEntry{}).Where("id = ?", entry.Id).Updates(map[string]any{
		"note":       note,
		"updated_at": now.Unix(),
	}).Error; err != nil {
		return nil, err
	}
	entry.Note, entry.UpdatedAt = note, now.Unix()
	return wtAllowlistItem(entry)
}

// checkWtAllowlistStatusChange refuses removing the owner and restoring an
// entry that is not removed.
func checkWtAllowlistStatusChange(entry *WtAllowlistEntry, account *User, status string) error {
	if status == WtAllowlistActive && entry.Status != WtAllowlistRemoved {
		return ErrWtAllowlistNotRemoved
	}
	if status == WtAllowlistRemoved {
		if entry.DiscordId != nil && *entry.DiscordId == WtOwnerDiscordID() {
			return ErrWtOwnerRemoval
		}
		if account != nil && account.Role == common.RoleRootUser {
			return ErrWtOwnerRemoval
		}
	}
	return nil
}

// WtAllowlistStatusAccount checks that an entry can move to status (removed
// or active) and returns the account the change disables or enables, 0 when
// there is none. The caller gets the owner's step-up verification for that
// account and passes the ID on to WtRemoveAllowlistEntry or
// WtRestoreAllowlistEntry, which refuse the change if the account changed in
// between.
func WtAllowlistStatusAccount(id int, status string) (int, error) {
	var entry WtAllowlistEntry
	if err := DB.First(&entry, id).Error; err != nil {
		return 0, wtNotFound(err, ErrWtAllowlistNotFound)
	}
	account, err := wtAllowlistAccount(DB, &entry)
	if err != nil {
		return 0, err
	}
	if err := checkWtAllowlistStatusChange(&entry, account, status); err != nil {
		return 0, err
	}
	if account == nil {
		return 0, nil
	}
	return account.Id, nil
}

// WtRemoveAllowlistEntry marks an entry removed and disables its account the
// way upstream user management disables a user: the auth version advances and
// browser sessions are revoked. In the same transaction the member's access
// tokens are deleted and its API keys switched off, so restoring the entry
// later gives back the account but none of them. accountID is the account the
// owner verified for (see WtAllowlistStatusAccount).
func WtRemoveAllowlistEntry(id, accountID int, now time.Time) (*WtAllowlistItem, WtRevokedCredentials, error) {
	return setWtAllowlistStatus(id, accountID, WtAllowlistRemoved, now)
}

// WtRestoreAllowlistEntry makes a removed entry active again and re-enables
// its account with the balance untouched; API keys the removal switched off
// stay off until the member switches them on. When the account was deleted
// the entry is unlinked, and the next sign-in creates a new account.
func WtRestoreAllowlistEntry(id, accountID int, now time.Time) (*WtAllowlistItem, error) {
	item, _, err := setWtAllowlistStatus(id, accountID, WtAllowlistActive, now)
	return item, err
}

func setWtAllowlistStatus(id, accountID int, status string, now time.Time) (*WtAllowlistItem, WtRevokedCredentials, error) {
	userStatus := common.UserStatusEnabled
	if status == WtAllowlistRemoved {
		userStatus = common.UserStatusDisabled
	}
	var entry WtAllowlistEntry
	var account *User
	var revoked WtRevokedCredentials
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&entry, id).Error; err != nil {
			return wtNotFound(err, ErrWtAllowlistNotFound)
		}
		var err error
		if account, err = wtAllowlistAccount(lockForUpdate(tx), &entry); err != nil {
			return err
		}
		if err := checkWtAllowlistStatusChange(&entry, account, status); err != nil {
			return err
		}
		linkedID := 0
		if account != nil {
			linkedID = account.Id
		}
		if linkedID != accountID {
			return ErrWtAllowlistChanged
		}
		// The entry changes first, so that enabling the account passes the
		// allowlist check of User.UpdateWithTx.
		entry.Status, entry.UserId, entry.UpdatedAt = status, linkedID, now.Unix()
		if err := tx.Model(&WtAllowlistEntry{}).Where("id = ?", entry.Id).Updates(map[string]any{
			"status":     entry.Status,
			"user_id":    entry.UserId,
			"updated_at": entry.UpdatedAt,
		}).Error; err != nil {
			return err
		}
		if account == nil {
			return nil
		}
		account.Status = userStatus
		if err := account.UpdateWithTx(tx, false); err != nil {
			return err
		}
		if status != WtAllowlistRemoved {
			return nil
		}
		revoked, err = revokeWtAccessWithTx(tx, account.Id)
		return err
	})
	if err != nil {
		return nil, revoked, err
	}
	if account != nil {
		// Same follow-up as User.Update and ManageUser: publish the new auth
		// state, revoke browser sessions and drop cached API tokens. Removal
		// repeats the revocation even when the account was already disabled,
		// and goes on when the cache cannot be written, because the database
		// already holds the new auth version.
		if err := updateUserCache(*account); err != nil {
			common.SysError(common.LogText("failed to update the cache of user %d: %s", account.Id, err.Error()))
		}
		if status == WtAllowlistRemoved {
			if _, err := RevokeAllUserSessions(account.Id, "wt_allowlist_removed"); err != nil {
				return nil, revoked, err
			}
		}
		if err := InvalidateUserTokensCache(account.Id); err != nil {
			common.SysError(common.LogText("failed to invalidate tokens cache for user %d: %s", account.Id, err.Error()))
		}
	}
	item, err := wtAllowlistItem(entry)
	return item, revoked, err
}

// WtListEntryRequests lists entry requests, newest first; status filters by
// state when set.
func WtListEntryRequests(status string, startIdx, num int) ([]WtEntryRequestItem, int64, error) {
	query := DB.Model(&WtEntryRequest{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var requests []WtEntryRequest
	if err := query.Order("requested_at DESC").Order("id DESC").Limit(num).Offset(startIdx).Find(&requests).Error; err != nil {
		return nil, 0, err
	}
	items, err := wtEntryRequestItems(requests)
	return items, total, err
}

// WtCountPendingEntryRequests counts entry requests waiting for the owner.
func WtCountPendingEntryRequests() (int64, error) {
	var count int64
	err := DB.Model(&WtEntryRequest{}).Where("status = ?", WtEntryPending).Count(&count).Error
	return count, err
}

// WtApproveEntryRequest admits the applicant: it adds an active allowlist
// entry for the Discord ID and marks the request approved. An active username
// pre-fill that names the applicant and has no Discord ID yet becomes that
// entry instead, so it no longer admits whoever signs in with the name next.
// A rejected request can still be approved.
func WtApproveEntryRequest(id int, now time.Time) (*WtEntryRequestItem, error) {
	var request WtEntryRequest
	err := DB.Transaction(func(tx *gorm.DB) error {
		var target WtEntryRequest
		if err := tx.First(&target, id).Error; err != nil {
			return wtNotFound(err, ErrWtEntryRequestNotFound)
		}
		if target.DiscordId == WtOwnerDiscordID() {
			return ErrWtAllowlistOwner
		}
		// Allowlist before the request: the lock order of the sign-in gate.
		entry, err := findWtAllowlistEntryWithTx(tx, target.DiscordId)
		if err != nil {
			return err
		}
		var prefill *WtAllowlistEntry
		if entry == nil {
			if prefill, err = findWtPrefillWithTx(tx, target.DiscordUsername); err != nil {
				return err
			}
		}
		if err := lockForUpdate(tx).First(&request, id).Error; err != nil {
			return wtNotFound(err, ErrWtEntryRequestNotFound)
		}
		if request.Status == WtEntryApproved {
			return ErrWtEntryRequestDecided
		}
		if entry != nil && entry.Status != WtAllowlistActive {
			return ErrWtApplicantRemoved
		}
		pinned := false
		if prefill != nil && wtUsernameKey(request.DiscordUsername) == wtUsernameKey(target.DiscordUsername) {
			if pinned, err = pinWtPrefillWithTx(tx, prefill, request.DiscordId, request.DiscordUsername, now); err != nil {
				return err
			}
		}
		if entry == nil && !pinned {
			var user User
			if err := tx.Select("id").Where("discord_id = ?", request.DiscordId).Order("id").Limit(1).Find(&user).Error; err != nil {
				return err
			}
			discordID := request.DiscordId
			if err := tx.Create(&WtAllowlistEntry{
				DiscordId:       &discordID,
				DiscordUsername: request.DiscordUsername,
				Status:          WtAllowlistActive,
				UserId:          user.Id,
				CreatedAt:       now.Unix(),
				UpdatedAt:       now.Unix(),
			}).Error; err != nil {
				return err
			}
		}
		request.Status, request.DecidedAt = WtEntryApproved, now.Unix()
		return tx.Model(&WtEntryRequest{}).Where("id = ?", request.Id).Updates(map[string]any{
			"status":     request.Status,
			"decided_at": request.DecidedAt,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	items, err := wtEntryRequestItems([]WtEntryRequest{request})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

// WtRejectEntryRequest rejects a pending entry request; the applicant can
// apply again WtEntryRetryDelay after the decision.
func WtRejectEntryRequest(id int, now time.Time) (*WtEntryRequestItem, error) {
	var request WtEntryRequest
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&request, id).Error; err != nil {
			return wtNotFound(err, ErrWtEntryRequestNotFound)
		}
		if request.Status != WtEntryPending {
			return ErrWtEntryRequestDecided
		}
		request.Status, request.DecidedAt = WtEntryRejected, now.Unix()
		return tx.Model(&WtEntryRequest{}).Where("id = ?", request.Id).Updates(map[string]any{
			"status":     request.Status,
			"decided_at": request.DecidedAt,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	items, err := wtEntryRequestItems([]WtEntryRequest{request})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}
