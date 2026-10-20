package model

import "strings"

// wind-tide: the WorkBuddy pool (wind-tide/API.md §16): donated Tencent
// CodeBuddy and WorkBuddy accounts held by the workbuddy2api panel. They
// share wt_donations with the other pools; the panel's account id is the
// credential name and the account's nickname is what the member sees. The
// pool has no call ledger: every member may use its channel.

const (
	// WtWbProvider marks the WorkBuddy rows of wt_donations.
	WtWbProvider = "workbuddy"

	WtPoolEventWbDonated = "wb_account_donated"
	WtPoolEventWbRemoved = "wb_account_removed"
)

// CreateWtWbDonation records a sign-in the member started as verifying.
// state is the panel's sign-in session, kept until the sign-in finishes
// and never sent to the browser; the account id comes with the sign-in.
func CreateWtWbDonation(userID int, state string) (*WtDonation, error) {
	now := WtNow().Unix()
	donation := &WtDonation{
		UserId:      userID,
		Provider:    WtWbProvider,
		Status:      WtDonationVerifying,
		CpaState:    state,
		StartedAt:   now,
		SubmittedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := DB.Create(donation).Error; err != nil {
		return nil, err
	}
	return donation, nil
}

// BindWtWbDonation marks a verifying donation active with the account the
// panel added: its id and its nickname, cut to the display length.
func BindWtWbDonation(id int, uid, nickname string) error {
	return wtSettleDonation(id, map[string]any{
		"status":          WtDonationActive,
		"credential_name": uid,
		"nickname":        wtTruncateDisplayName(strings.TrimSpace(nickname)),
		"verified_at":     WtNow().Unix(),
		"cpa_state":       "",
		"fail_reason":     "",
	})
}
