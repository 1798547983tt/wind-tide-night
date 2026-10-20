package model

import (
	"net/mail"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// wind-tide: the ledger of donated accounts: which member donated which
// credential of the CPA account pool (Antigravity, wind-tide/API.md §14) or
// which account of the aistudio-api pool (AI Studio, §15 and wt_ais.go). The
// Google tokens and cookies themselves live only in the pools.

const (
	AuthFlowPurposeWtDonation = "wt_donation"
	AuthFlowIntentWtDonate    = "donate"

	WtDonationVerifying = "verifying"
	WtDonationActive    = "active"
	WtDonationFailed    = "failed"
	WtDonationRemoved   = "removed"

	// WtDonationMaxPerUser is how many verifying and active donations one
	// member may hold.
	WtDonationMaxPerUser = 10

	wtDonationEmailMax = 254
)

var (
	ErrWtDonationNotFound  = common.NewMessage("Donation does not exist")
	ErrWtDonationEmail     = common.NewMessage("Enter a valid email address")
	ErrWtDonationLimit     = common.NewMessage("You can donate at most {{max}} accounts", map[string]any{"max": WtDonationMaxPerUser})
	ErrWtDonationDuplicate = common.NewMessage("This account is already in the pool")

	wtDonationSlotStates = []string{WtDonationVerifying, WtDonationActive}
)

// WtDonation is one donated account. Email is lower case and may change
// after verification to the account Google actually authorized; Nickname is
// what a WorkBuddy account goes by (wt_wb.go). CpaState is the authorization
// session while verifying, cleared afterwards. All times are Unix seconds; 0
// means "not yet".
type WtDonation struct {
	Id                int    `json:"id"`
	UserId            int    `json:"user_id" gorm:"not null;index:idx_wt_donations_user_id"`
	Provider          string `json:"provider" gorm:"type:varchar(32);not null"`
	Email             string `json:"email" gorm:"type:varchar(255);not null;index:idx_wt_donations_email"`
	Nickname          string `json:"nickname" gorm:"type:varchar(64);not null;default:''"`
	CredentialName    string `json:"credential_name" gorm:"type:varchar(255);not null"`
	AuthIndex         string `json:"auth_index" gorm:"type:varchar(64);not null"`
	ProjectId         string `json:"project_id" gorm:"type:varchar(128);not null"`
	Status            string `json:"status" gorm:"type:varchar(16);not null;index:idx_wt_donations_status"`
	FailReason        string `json:"fail_reason" gorm:"type:varchar(255);not null"`
	CpaState          string `json:"-" gorm:"type:varchar(128);not null"`
	StartedAt         int64  `json:"started_at" gorm:"type:bigint;not null"`
	SubmittedAt       int64  `json:"submitted_at" gorm:"type:bigint;not null"`
	VerifiedAt        int64  `json:"verified_at" gorm:"type:bigint;not null"`
	RemovedAt         int64  `json:"removed_at" gorm:"type:bigint;not null"`
	LastTestAt        int64  `json:"last_test_at" gorm:"type:bigint;not null"`
	LastTestOk        int    `json:"last_test_ok" gorm:"not null"`
	LastTestLatencyMs int    `json:"last_test_latency_ms" gorm:"not null"`
	LastTestMessage   string `json:"last_test_message" gorm:"type:varchar(255);not null"`
	TestsDay          string `json:"tests_day" gorm:"type:varchar(10);not null"`
	TestsToday        int    `json:"tests_today" gorm:"not null"`
	CreatedAt         int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt         int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (WtDonation) TableName() string {
	return "wt_donations"
}

// WtNormalizeDonationEmail lower-cases a bare address and reports whether it
// is one: no display name, at most 254 characters, a dot in the domain.
func WtNormalizeDonationEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > wtDonationEmailMax {
		return "", false
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return "", false
	}
	_, domain, _ := strings.Cut(email, "@")
	if !strings.Contains(domain, ".") {
		return "", false
	}
	return email, true
}

// CountWtDonationSlots counts the donations of a member to one pool that
// take one of its slots: the verifying and the active ones. Each provider
// has its own ten slots.
func CountWtDonationSlots(userID int, provider string) (int64, error) {
	var count int64
	err := DB.Model(&WtDonation{}).Where("user_id = ? AND provider = ? AND status IN ?", userID, provider, wtDonationSlotStates).Count(&count).Error
	return count, err
}

// WtDonationEmailInPool reports whether any member has a verifying or active
// donation of email in the provider's pool, other than exceptID.
func WtDonationEmailInPool(email, provider string, exceptID int) (bool, error) {
	var count int64
	err := DB.Model(&WtDonation{}).Where("email = ? AND provider = ? AND id <> ? AND status IN ?", email, provider, exceptID, wtDonationSlotStates).Count(&count).Error
	return count > 0, err
}

// WtDonationCredentialClaimed reports whether a verifying or active donation
// of the provider other than exceptID already refers to the credential name.
func WtDonationCredentialClaimed(name, provider string, exceptID int) (bool, error) {
	var count int64
	err := DB.Model(&WtDonation{}).Where("credential_name = ? AND provider = ? AND id <> ? AND status IN ?", name, provider, exceptID, wtDonationSlotStates).Count(&count).Error
	return count > 0, err
}

// GetWtDonation loads one donation of a member. Another member's donation
// does not exist as far as the caller can tell.
func GetWtDonation(id, userID int) (*WtDonation, error) {
	var donation WtDonation
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&donation).Error; err != nil {
		return nil, wtNotFound(err, ErrWtDonationNotFound)
	}
	return &donation, nil
}

// ListWtDonations lists a member's donations to one pool that are not
// removed, newest first.
func ListWtDonations(userID int, provider string) ([]*WtDonation, error) {
	var donations []*WtDonation
	err := DB.Where("user_id = ? AND provider = ? AND status <> ?", userID, provider, WtDonationRemoved).Order("created_at DESC, id DESC").Find(&donations).Error
	return donations, err
}

// ListVerifyingWtDonations lists every donation to one pool still being
// verified, for the pass a restarted server makes over them.
func ListVerifyingWtDonations(provider string) ([]*WtDonation, error) {
	var donations []*WtDonation
	err := DB.Where("provider = ? AND status = ?", provider, WtDonationVerifying).Order("id ASC").Find(&donations).Error
	return donations, err
}

// ListActiveWtDonations lists every active donation to one pool, oldest
// first, for the pass that checks them against the pool.
func ListActiveWtDonations(provider string) ([]*WtDonation, error) {
	var donations []*WtDonation
	err := DB.Where("provider = ? AND status = ?", provider, WtDonationActive).Order("id ASC").Find(&donations).Error
	return donations, err
}

// StartWtDonationVerification records a submitted callback inside tx: a new
// verifying donation when donation.Id is 0, otherwise the member's existing
// active or failed donation back to verifying (a re-authorization). The
// donation is updated in place for the caller.
func StartWtDonationVerification(tx *gorm.DB, donation *WtDonation, state string, startedAt int64) error {
	now := WtNow().Unix()
	if donation.Id == 0 {
		donation.Status = WtDonationVerifying
		donation.CpaState = state
		donation.StartedAt = startedAt
		donation.SubmittedAt = now
		donation.CreatedAt, donation.UpdatedAt = now, now
		return tx.Create(donation).Error
	}
	result := tx.Model(&WtDonation{}).
		Where("id = ? AND user_id = ? AND status IN ?", donation.Id, donation.UserId, []string{WtDonationActive, WtDonationFailed}).
		Updates(map[string]any{
			"status":       WtDonationVerifying,
			"cpa_state":    state,
			"started_at":   startedAt,
			"submitted_at": now,
			"fail_reason":  "",
			"updated_at":   now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrWtDonationNotFound
	}
	donation.Status, donation.CpaState, donation.StartedAt, donation.SubmittedAt, donation.FailReason = WtDonationVerifying, state, startedAt, now, ""
	return nil
}

// wtSettleDonation applies fields to a donation that is still verifying.
func wtSettleDonation(id int, fields map[string]any) error {
	fields["updated_at"] = WtNow().Unix()
	result := DB.Model(&WtDonation{}).Where("id = ? AND status = ?", id, WtDonationVerifying).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrWtDonationNotFound
	}
	return nil
}

// BindWtDonation marks a verifying donation active with the credential the
// pool holds for it.
func BindWtDonation(id int, credentialName, authIndex, projectID, email string) error {
	return wtSettleDonation(id, map[string]any{
		"status":          WtDonationActive,
		"credential_name": credentialName,
		"auth_index":      authIndex,
		"project_id":      projectID,
		"email":           email,
		"verified_at":     WtNow().Unix(),
		"cpa_state":       "",
		"fail_reason":     "",
	})
}

// FailWtDonation records why a verifying donation failed. The credential name
// stays, so removing the donation still deletes the file of an earlier
// authorization.
func FailWtDonation(id int, reason string) error {
	return wtSettleDonation(id, map[string]any{
		"status":      WtDonationFailed,
		"fail_reason": reason,
		"cpa_state":   "",
	})
}

// FailActiveWtDonation records why the pool stopped using an active donation
// once the account is out of the pool; the member reads the reason and may
// donate the account afresh. A donation that is no longer active is left
// alone and reported as ErrWtDonationNotFound.
func FailActiveWtDonation(id int, reason string) error {
	now := WtNow().Unix()
	result := DB.Model(&WtDonation{}).Where("id = ? AND status = ?", id, WtDonationActive).Updates(map[string]any{
		"status":      WtDonationFailed,
		"fail_reason": reason,
		"cpa_state":   "",
		"updated_at":  now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrWtDonationNotFound
	}
	return nil
}

// RemoveWtDonation closes a donation after CPA deleted its credential.
func RemoveWtDonation(id int) error {
	now := WtNow().Unix()
	return DB.Model(&WtDonation{}).Where("id = ?", id).Updates(map[string]any{
		"status":     WtDonationRemoved,
		"removed_at": now,
		"cpa_state":  "",
		"updated_at": now,
	}).Error
}

// RecordWtDonationTest stores the outcome of a test and the day's count.
func RecordWtDonationTest(id int, ok bool, latencyMs int, message, day string, testsToday int) error {
	okValue := 0
	if ok {
		okValue = 1
	}
	now := WtNow().Unix()
	return DB.Model(&WtDonation{}).Where("id = ?", id).Updates(map[string]any{
		"last_test_at":         now,
		"last_test_ok":         okValue,
		"last_test_latency_ms": latencyMs,
		"last_test_message":    message,
		"tests_day":            day,
		"tests_today":          testsToday,
		"updated_at":           now,
	}).Error
}
