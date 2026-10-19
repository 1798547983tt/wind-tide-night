package service

import (
	"context"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

// wind-tide: the verification of a donated AI Studio account (wind-tide/API.md
// §15). After the pool imported the cookies, the site asks it to sign the
// account in and read its email; the donation is active once the email is
// known and not already in the pool.

// The reasons an AI Studio verification can end with.
const (
	WtAisFailDuplicate = "This account is already in the pool"
	WtAisFailGone      = "The pool lost the account; donate again"
	WtAisFailTimeout   = "The pool could not read the account in time; try again later"
)

// Tests shorten the polling; WtDonationVerifySync makes the job run inline.
var (
	WtAisVerifyInterval = 10 * time.Second
	WtAisVerifyAttempts = 12
)

var wtAisVerifyOnce sync.Once

// WtAisVerifyJob is one imported account to verify. DisplayName is what the
// announcement shows when the account joins the pool.
type WtAisVerifyJob struct {
	DonationID  int
	UserID      int
	AccountID   string
	DisplayName string
}

// WtStartAisVerification verifies an imported account in the background,
// inline when WtDonationVerifySync is set. A job already running for the
// same donation is left alone.
func WtStartAisVerification(job WtAisVerifyJob) {
	if _, running := wtDonationInflight.LoadOrStore(job.DonationID, struct{}{}); running {
		return
	}
	run := func() {
		defer wtDonationInflight.Delete(job.DonationID)
		wtVerifyAisDonation(job)
	}
	if WtDonationVerifySync {
		run()
		return
	}
	gopool.Go(run)
}

// wtVerifyAisDonation asks the pool for the account's email every
// WtAisVerifyInterval, up to WtAisVerifyAttempts times. A page the pool
// could not read, a browser that is not ready and an unreachable pool are
// retried; a lost account and the last attempt fail the donation.
func wtVerifyAisDonation(job WtAisVerifyJob) {
	ctx := context.Background()
	for attempt := range WtAisVerifyAttempts {
		if attempt > 0 {
			time.Sleep(WtAisVerifyInterval)
		}
		outcome, email, err := WtAisDetectEmail(ctx, job.AccountID)
		switch {
		case err == nil && outcome == WtAisDetectOK:
			wtBindAisDonation(ctx, job, email)
			return
		case err == nil && outcome == WtAisDetectGone:
			wtFailAisDonation(ctx, job, WtAisFailGone)
			return
		}
	}
	wtFailAisDonation(ctx, job, WtAisFailTimeout)
}

// wtBindAisDonation marks the donation active with the email the pool read,
// unless another donation already brought that account in.
func wtBindAisDonation(ctx context.Context, job WtAisVerifyJob, email string) {
	normalized, valid := model.WtNormalizeDonationEmail(email)
	if !valid {
		wtFailAisDonation(ctx, job, WtAisFailTimeout)
		return
	}
	claimed, err := model.WtDonationEmailInPool(normalized, model.WtAisProvider, job.DonationID)
	if err != nil {
		common.SysError(common.LogText("wt ais: donation %d could not be checked for conflicts: %s", job.DonationID, err.Error()))
		return
	}
	if claimed {
		wtFailAisDonation(ctx, job, WtAisFailDuplicate)
		return
	}
	if err := model.BindWtAisDonation(job.DonationID, normalized); err != nil {
		common.SysError(common.LogText("wt ais: donation %d could not be marked active: %s", job.DonationID, err.Error()))
		return
	}
	WtForgetAisRotation()
	if err := model.WtInsertPoolEvent(model.WtPoolEventAisDonated, model.WtAisProvider, job.UserID, job.DisplayName, 1, ""); err != nil {
		common.SysError(common.LogText("wt ais: the announcement of donation %d was not saved: %s", job.DonationID, err.Error()))
	}
	model.RecordLog(job.UserID, model.LogTypeSystem, common.NewMessage("Donated account #{{id}} joined the pool", map[string]any{"id": job.DonationID}))
}

// wtFailAisDonation takes the account out of the pool, records the reason
// and tells the member. The pool refusing to delete is not retried.
func wtFailAisDonation(ctx context.Context, job WtAisVerifyJob, reason string) {
	if err := WtAisDeleteAccount(ctx, job.AccountID); err != nil {
		common.SysError(common.LogText("wt ais: the account of failed donation %d was not deleted from the pool", job.DonationID))
	}
	if err := model.FailWtDonation(job.DonationID, reason); err != nil {
		common.SysError(common.LogText("wt ais: donation %d could not be marked failed: %s", job.DonationID, err.Error()))
		return
	}
	model.RecordLog(job.UserID, model.LogTypeSystem, common.NewMessage("Donation #{{id}} failed", map[string]any{"id": job.DonationID}))
}

// StartWtAisVerifyTask verifies, once per process and only on the master
// node, the AI Studio donations a restart left verifying.
func StartWtAisVerifyTask() {
	wtAisVerifyOnce.Do(func() {
		if !common.IsMasterNode || !WtAisEnabled() {
			return
		}
		gopool.Go(WtSettleVerifyingAisDonations)
	})
}

// WtSettleVerifyingAisDonations is the pass StartWtAisVerifyTask makes: every
// verifying AI Studio donation is verified again, one after the other so the
// pool's browser is not asked for two accounts at once.
func WtSettleVerifyingAisDonations() {
	donations, err := model.ListVerifyingWtDonations(model.WtAisProvider)
	if err != nil {
		common.SysError(common.LogText("wt ais: verifying donations could not be listed: %s", err.Error()))
		return
	}
	for _, donation := range donations {
		if _, running := wtDonationInflight.LoadOrStore(donation.Id, struct{}{}); running {
			continue
		}
		wtVerifyAisDonation(WtAisVerifyJob{
			DonationID:  donation.Id,
			UserID:      donation.UserId,
			AccountID:   donation.CredentialName,
			DisplayName: model.WtDisplayNameOfUser(donation.UserId),
		})
		wtDonationInflight.Delete(donation.Id)
	}
}
