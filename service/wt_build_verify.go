package service

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

// wind-tide: the verification of a donated Build account (wind-tide/API.md
// §15). The console has no email probe: after the auth file is uploaded the
// site watches the console's status until the account holds a browser
// context (it signed in) or is marked expired (the sign-in bounced to
// Google's login page). An account the console has not tried yet is asked
// for by a switch half-way through; one still untried at the end is taken
// on trust, as the console marks it expired the first time it is used and
// the list shows that.

// The reasons a Build verification can end with.
const (
	WtBuildFailExpired = "The pool could not sign in with this account; export the Cookie again and donate it afresh"
	WtBuildFailGone    = "The pool lost the account; donate again"
)

// Tests shorten the polling; WtDonationVerifySync makes the job run inline.
var (
	WtBuildVerifyInterval = 10 * time.Second
	WtBuildVerifyAttempts = 12
	// wtBuildVerifySwitchAt is the attempt after which an account without a
	// context is asked for by a switch.
	wtBuildVerifySwitchAt = 4
)

var wtBuildVerifyOnce sync.Once

// WtBuildVerifyJob is one uploaded account to verify. DisplayName is what
// the announcement shows when the account joins the pool.
type WtBuildVerifyJob struct {
	DonationID  int
	UserID      int
	Index       int
	DisplayName string
}

// WtStartBuildVerification verifies an uploaded account in the background,
// inline when WtDonationVerifySync is set. A job already running for the
// same donation is left alone.
func WtStartBuildVerification(job WtBuildVerifyJob) {
	if _, running := wtDonationInflight.LoadOrStore(job.DonationID, struct{}{}); running {
		return
	}
	run := func() {
		defer wtDonationInflight.Delete(job.DonationID)
		wtVerifyBuildDonation(job)
	}
	if WtDonationVerifySync {
		run()
		return
	}
	gopool.Go(run)
}

// wtVerifyBuildDonation reads the console's status every
// WtBuildVerifyInterval, up to WtBuildVerifyAttempts times. A context on the
// account binds the donation, an expired or unreadable file fails it, a lost
// file fails it, an unreachable console is retried, and an account the
// console never tried is bound at the end.
func wtVerifyBuildDonation(job WtBuildVerifyJob) {
	ctx := context.Background()
	for attempt := range WtBuildVerifyAttempts {
		if attempt > 0 {
			time.Sleep(WtBuildVerifyInterval)
		}
		status, err := WtBuildStatusNow(ctx)
		if err != nil {
			continue
		}
		account := status.Account(job.Index)
		switch {
		case account == nil:
			wtFailBuildDonation(ctx, job, WtBuildFailGone)
			return
		case account.IsExpired || account.IsInvalid:
			wtFailBuildDonation(ctx, job, WtBuildFailExpired)
			return
		case account.HasContext:
			wtBindBuildDonation(job)
			return
		case attempt == wtBuildVerifySwitchAt && !status.Busy:
			if err := WtBuildSwitchTo(ctx, job.Index); err != nil {
				common.SysError(common.LogText("wt build: donation %d could not be switched to for its check: %s", job.DonationID, err.Error()))
			}
		}
	}
	wtBindBuildDonation(job)
}

// wtBindBuildDonation marks the donation active; the console reads no email,
// so the record keeps none.
func wtBindBuildDonation(job WtBuildVerifyJob) {
	if err := model.BindWtAisDonation(job.DonationID, ""); err != nil {
		common.SysError(common.LogText("wt build: donation %d could not be marked active: %s", job.DonationID, err.Error()))
		return
	}
	WtForgetBuildStatus()
	if err := model.WtInsertPoolEvent(model.WtPoolEventBuildDonated, model.WtBuildProvider, job.UserID, job.DisplayName, 1, ""); err != nil {
		common.SysError(common.LogText("wt build: the announcement of donation %d was not saved: %s", job.DonationID, err.Error()))
	}
	model.RecordLog(job.UserID, model.LogTypeSystem, common.NewMessage("Donated account #{{id}} joined the pool", map[string]any{"id": job.DonationID}))
}

// wtFailBuildDonation takes the auth file out of the console, records the
// reason and tells the member. The console refusing to delete is not retried.
func wtFailBuildDonation(ctx context.Context, job WtBuildVerifyJob, reason string) {
	if err := WtBuildDeleteAccount(ctx, job.Index); err != nil {
		common.SysError(common.LogText("wt build: the account of failed donation %d was not deleted from the pool", job.DonationID))
	}
	if err := model.FailWtDonation(job.DonationID, reason); err != nil {
		common.SysError(common.LogText("wt build: donation %d could not be marked failed: %s", job.DonationID, err.Error()))
		return
	}
	WtForgetBuildStatus()
	model.RecordLog(job.UserID, model.LogTypeSystem, common.NewMessage("Donation #{{id}} failed", map[string]any{"id": job.DonationID}))
}

// StartWtBuildVerifyTask verifies, once per process and only on the master
// node, the Build donations a restart left verifying.
func StartWtBuildVerifyTask() {
	wtBuildVerifyOnce.Do(func() {
		if !common.IsMasterNode || !WtBuildEnabled() {
			return
		}
		gopool.Go(WtSettleVerifyingBuildDonations)
	})
}

// WtSettleVerifyingBuildDonations is the pass StartWtBuildVerifyTask makes:
// every verifying Build donation is verified again, one after the other so
// the console is not asked to switch twice at once.
func WtSettleVerifyingBuildDonations() {
	donations, err := model.ListVerifyingWtDonations(model.WtBuildProvider)
	if err != nil {
		common.SysError(common.LogText("wt build: verifying donations could not be listed: %s", err.Error()))
		return
	}
	for _, donation := range donations {
		index, convErr := strconv.Atoi(donation.CredentialName)
		if convErr != nil {
			continue
		}
		if _, running := wtDonationInflight.LoadOrStore(donation.Id, struct{}{}); running {
			continue
		}
		wtVerifyBuildDonation(WtBuildVerifyJob{
			DonationID:  donation.Id,
			UserID:      donation.UserId,
			Index:       index,
			DisplayName: model.WtDisplayNameOfUser(donation.UserId),
		})
		wtDonationInflight.Delete(donation.Id)
	}
}
