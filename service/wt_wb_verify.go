package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

// wind-tide: the verification of a donated WorkBuddy account (wind-tide/API.md
// §16). The member signs in on Tencent's page; the site asks the panel every
// few seconds whether the sign-in finished. When it has, the panel already
// holds the account: the site checks that no other donation claims the same
// account, then binds the record to it. A sign-in nobody finishes within the
// panel's quarter of an hour fails.

// The reasons a WorkBuddy verification can end with.
const (
	WtWbFailTimeout   = "The sign-in was not finished in time; start again"
	WtWbFailDuplicate = "This account is already in the pool"
)

// Tests shorten the polling; WtDonationVerifySync makes the job run inline.
var (
	WtWbVerifyInterval = 3 * time.Second
	WtWbVerifyAttempts = int(WtWbSignInTTL / (3 * time.Second))
)

var wtWbVerifyOnce sync.Once

// WtWbVerifyJob is one started sign-in to follow. DisplayName is what the
// announcement shows when the account joins the pool.
type WtWbVerifyJob struct {
	DonationID  int
	UserID      int
	State       string
	DisplayName string
}

// WtStartWbVerification follows a sign-in in the background, inline when
// WtDonationVerifySync is set. A job already running for the same donation
// is left alone.
func WtStartWbVerification(job WtWbVerifyJob) {
	if _, running := wtDonationInflight.LoadOrStore(job.DonationID, struct{}{}); running {
		return
	}
	run := func() {
		defer wtDonationInflight.Delete(job.DonationID)
		wtVerifyWbDonation(job)
	}
	if WtDonationVerifySync {
		run()
		return
	}
	gopool.Go(run)
}

// wtVerifyWbDonation polls the sign-in every WtWbVerifyInterval, up to
// WtWbVerifyAttempts times. A finished sign-in binds the donation or fails
// it as a duplicate, a sign-in the panel forgot fails it, a panel that does
// not answer is asked again, and the attempts running out fail it.
func wtVerifyWbDonation(job WtWbVerifyJob) {
	ctx := context.Background()
	for attempt := range WtWbVerifyAttempts {
		if attempt > 0 {
			time.Sleep(WtWbVerifyInterval)
		}
		result, err := WtWbPollSignIn(ctx, job.State)
		switch {
		case errors.Is(err, ErrWtWbSignInExpired):
			wtFailWbDonation(job, WtWbFailTimeout)
			return
		case err != nil || !result.Done:
			continue
		}
		wtBindWbDonation(ctx, job, result)
		return
	}
	wtFailWbDonation(job, WtWbFailTimeout)
}

// wtBindWbDonation marks the donation active with the account the panel
// added. An account another donation already holds is not taken: the panel
// refreshed that account's token and the new record fails as a duplicate.
// A record that is no longer verifying (the member removed it meanwhile)
// takes the account back out of the panel.
func wtBindWbDonation(ctx context.Context, job WtWbVerifyJob, result *WtWbSignInResult) {
	claimed, err := model.WtDonationCredentialClaimed(result.UID, model.WtWbProvider, job.DonationID)
	if err != nil {
		common.SysError(common.LogText("wt wb: donation %d could not be checked against the pool: %s", job.DonationID, err.Error()))
		return
	}
	if claimed {
		wtFailWbDonation(job, WtWbFailDuplicate)
		return
	}
	if err := model.BindWtWbDonation(job.DonationID, result.UID, result.Nickname); err != nil {
		common.SysError(common.LogText("wt wb: donation %d could not be marked active: %s", job.DonationID, err.Error()))
		if removeErr := WtWbRemoveAccount(ctx, result.UID); removeErr != nil {
			common.SysError(common.LogText("wt wb: the account of closed donation %d was not removed from the pool", job.DonationID))
		}
		return
	}
	WtForgetWbStatus()
	if err := model.WtInsertPoolEvent(model.WtPoolEventWbDonated, model.WtWbProvider, job.UserID, job.DisplayName, 1, ""); err != nil {
		common.SysError(common.LogText("wt wb: the announcement of donation %d was not saved: %s", job.DonationID, err.Error()))
	}
	model.RecordLog(job.UserID, model.LogTypeSystem, common.NewMessage("Donated account #{{id}} joined the pool", map[string]any{"id": job.DonationID}))
}

// wtFailWbDonation records why the sign-in failed and tells the member. The
// panel holds nothing for a failed sign-in, so there is nothing to delete.
func wtFailWbDonation(job WtWbVerifyJob, reason string) {
	if err := model.FailWtDonation(job.DonationID, reason); err != nil {
		common.SysError(common.LogText("wt wb: donation %d could not be marked failed: %s", job.DonationID, err.Error()))
		return
	}
	model.RecordLog(job.UserID, model.LogTypeSystem, common.NewMessage("Donation #{{id}} failed", map[string]any{"id": job.DonationID}))
}

// StartWtWbVerifyTask follows, once per process and only on the master
// node, the WorkBuddy sign-ins a restart left verifying.
func StartWtWbVerifyTask() {
	wtWbVerifyOnce.Do(func() {
		if !common.IsMasterNode || !WtWbEnabled() {
			return
		}
		gopool.Go(WtSettleVerifyingWbDonations)
	})
}

// WtSettleVerifyingWbDonations is the pass StartWtWbVerifyTask makes: every
// verifying WorkBuddy donation is followed again from its sign-in session;
// one without a session fails.
func WtSettleVerifyingWbDonations() {
	donations, err := model.ListVerifyingWtDonations(model.WtWbProvider)
	if err != nil {
		common.SysError(common.LogText("wt wb: verifying donations could not be listed: %s", err.Error()))
		return
	}
	for _, donation := range donations {
		job := WtWbVerifyJob{
			DonationID:  donation.Id,
			UserID:      donation.UserId,
			State:       donation.CpaState,
			DisplayName: model.WtDisplayNameOfUser(donation.UserId),
		}
		if job.State == "" {
			wtFailWbDonation(job, WtWbFailTimeout)
			continue
		}
		WtStartWbVerification(job)
	}
}
