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

// wind-tide: the pass that keeps the Cookie pools and the donation ledger in
// step (wind-tide/API.md §15.5). Google signs a session out when one Cookie
// is used from two places; the pools notice on their next request and flag
// the account (aistudio-api: session_expired, AIStudioToAPI: isExpired or
// isInvalid). Every minute the site reads those flags, takes the flagged
// accounts out of the pools, fails their donations with the reason the
// member needs, squares the day's ledger and announces it, so a dead account
// neither keeps earning its donor calls nor wastes the pool's rotation.

// WtPoolFailSignedOut is the reason of a Cookie donation whose account the
// pool can no longer sign in; WtBuildFailUnreadable that of an auth file the
// BUILD console cannot read any more.
const (
	WtPoolFailSignedOut   = WtBuildFailExpired
	WtBuildFailUnreadable = "The pool could not read this account; export the Cookie again and donate it afresh"
)

// WtPoolSyncInterval is how often the pass runs; tests call the pass directly.
var WtPoolSyncInterval = time.Minute

var wtPoolSyncOnce sync.Once

// StartWtPoolSyncTask runs the pass every WtPoolSyncInterval, once per
// process and only on the master node while a Cookie pool is wired.
func StartWtPoolSyncTask() {
	wtPoolSyncOnce.Do(func() {
		if !common.IsMasterNode || (!WtAisEnabled() && !WtBuildEnabled()) {
			return
		}
		gopool.Go(func() {
			for {
				time.Sleep(WtPoolSyncInterval)
				WtSyncCookiePools(context.Background())
			}
		})
	})
}

// WtSyncCookiePools is one pass over both Cookie pools. A pool that is not
// wired, does not answer or refuses to delete waits for the next pass; an
// account the pool no longer lists is left to its member, as the list shows
// it; a donation the member removed meanwhile is left to their removal.
func WtSyncCookiePools(ctx context.Context) {
	if WtAisEnabled() {
		donations, err := model.ListActiveWtDonations(model.WtAisProvider)
		if err != nil {
			common.SysError(common.LogText("wt ais: active donations could not be listed: %s", err.Error()))
		}
		var rotation map[string]*WtAisRotationEntry
		if len(donations) > 0 {
			rotation, _ = WtAisRotation(ctx)
		}
		for _, donation := range donations {
			entry := rotation[donation.CredentialName]
			if entry == nil || !entry.SessionExpired {
				continue
			}
			if err := WtAisDeleteAccount(ctx, donation.CredentialName); err != nil {
				common.SysError(common.LogText("wt ais: the signed-out account of donation %d could not be deleted from the pool: %s", donation.Id, err.Error()))
				continue
			}
			WtForgetAisRotation()
			wtRetireDonation(donation, WtPoolFailSignedOut, model.WtPoolEventAisExpired)
		}
	}
	if WtBuildEnabled() {
		donations, err := model.ListActiveWtDonations(model.WtBuildProvider)
		if err != nil {
			common.SysError(common.LogText("wt build: active donations could not be listed: %s", err.Error()))
		}
		var status *WtBuildStatus
		if len(donations) > 0 {
			status, _ = WtBuildStatusCached(ctx)
		}
		for _, donation := range donations {
			index, convErr := strconv.Atoi(donation.CredentialName)
			if convErr != nil {
				continue
			}
			account := status.Account(index)
			if account == nil || (!account.IsExpired && !account.IsInvalid) {
				continue
			}
			reason := WtPoolFailSignedOut
			if account.IsInvalid {
				reason = WtBuildFailUnreadable
			}
			if err := WtBuildDeleteAccount(ctx, index); err != nil {
				common.SysError(common.LogText("wt build: the signed-out account of donation %d could not be deleted from the pool: %s", donation.Id, err.Error()))
				continue
			}
			WtForgetBuildStatus()
			wtRetireDonation(donation, reason, model.WtPoolEventBuildExpired)
		}
	}
}

// wtRetireDonation closes an active donation whose account just left the
// pool: the reason is recorded for the member, the feed and the member's log
// say so, and the day's ledger is squared (model.WtAisSettleAccountLoss).
func wtRetireDonation(donation *model.WtDonation, reason, kind string) {
	if err := model.FailActiveWtDonation(donation.Id, reason); err != nil {
		return
	}
	displayName := model.WtDisplayNameOfUser(donation.UserId)
	if err := model.WtInsertPoolEvent(kind, donation.Provider, donation.UserId, displayName, 1, ""); err != nil {
		common.SysError(common.LogText("wt pool: the announcement of signed-out donation %d was not saved: %s", donation.Id, err.Error()))
	}
	if err := model.WtAisSettleAccountLoss(donation.UserId, donation.Provider, model.WtCheckinDay(model.WtNow()), displayName); err != nil {
		common.SysError(common.LogText("wt pool: the ledger of member %d was not settled after donation %d signed out: %s", donation.UserId, donation.Id, err.Error()))
	}
	model.RecordLog(donation.UserId, model.LogTypeSystem, common.NewMessage("Donated account #{{id}} signed out and left the pool; export its Cookie again to donate it afresh", map[string]any{"id": donation.Id}))
}
