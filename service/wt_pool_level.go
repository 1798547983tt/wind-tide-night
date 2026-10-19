package service

import (
	"context"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

// wind-tide: the level of the Antigravity tank on the pool page
// (wind-tide/API.md §15): how much allowance the whole CPA pool has left,
// averaged over every credential that answers, refreshed every five minutes
// in the background.

const (
	wtPoolLevelInterval = 5 * time.Minute
	wtPoolLevelParallel = 4
)

// WtPoolLevelBucket is one quota window averaged over the pool.
type WtPoolLevelBucket struct {
	Name                     string  `json:"name"`
	Window                   string  `json:"window"`
	AverageRemainingFraction float64 `json:"average_remaining_fraction"`
}

// WtPoolLevel is the last snapshot of the Antigravity pool; FetchedAt is 0
// before the first refresh.
type WtPoolLevel struct {
	FetchedAt int64               `json:"fetched_at"`
	Accounts  int                 `json:"accounts"`
	Healthy   int                 `json:"healthy"`
	Buckets   []WtPoolLevelBucket `json:"buckets"`
}

var (
	wtPoolLevelOnce sync.Once
	wtPoolLevelMu   sync.Mutex
	wtPoolLevel     = WtPoolLevel{Buckets: []WtPoolLevelBucket{}}
)

// WtPoolLevelSnapshot is the last refresh, or the empty snapshot.
func WtPoolLevelSnapshot() WtPoolLevel {
	wtPoolLevelMu.Lock()
	defer wtPoolLevelMu.Unlock()
	snapshot := wtPoolLevel
	snapshot.Buckets = append([]WtPoolLevelBucket{}, wtPoolLevel.Buckets...)
	return snapshot
}

// WtResetPoolLevel forgets the snapshot; tests call it between cases.
func WtResetPoolLevel() {
	wtPoolLevelMu.Lock()
	wtPoolLevel = WtPoolLevel{Buckets: []WtPoolLevelBucket{}}
	wtPoolLevelMu.Unlock()
}

// StartWtPoolLevelTask refreshes the snapshot every five minutes, once per
// process and only on the master node while the site is wired to a pool.
func StartWtPoolLevelTask() {
	wtPoolLevelOnce.Do(func() {
		if !common.IsMasterNode || !WtDonationEnabled() {
			return
		}
		gopool.Go(func() {
			for {
				WtRefreshPoolLevel(context.Background())
				time.Sleep(wtPoolLevelInterval)
			}
		})
	})
}

// WtRefreshPoolLevel lists every Antigravity credential of the pool, asks
// Google for each one's allowance (at most four at a time, through the
// usage cache) and stores the averages. A pool that does not answer leaves
// the previous snapshot in place.
func WtRefreshPoolLevel(ctx context.Context) {
	list, err := WtPoolCredentials(ctx)
	if err != nil {
		common.SysError(common.LogText("wt donation: the pool level could not be refreshed: %s", err.Error()))
		return
	}
	now := model.WtNow()
	credentials := make([]WtCPACredential, 0, len(list))
	healthy := 0
	for _, entry := range list {
		if entry.Provider != "" && entry.Provider != WtDonationProvider {
			continue
		}
		credentials = append(credentials, entry)
		if WtPoolStateOf(&entry, now).State == WtPoolOK {
			healthy++
		}
	}
	usages := make([]*WtUsage, len(credentials))
	slots := make(chan struct{}, wtPoolLevelParallel)
	var group sync.WaitGroup
	for i := range credentials {
		if credentials[i].AuthIndex == "" || credentials[i].ProjectID == "" {
			continue
		}
		slots <- struct{}{}
		group.Go(func() {
			defer func() { <-slots }()
			usage, _, err := WtCredentialUsage(ctx, credentials[i].AuthIndex, credentials[i].ProjectID, false)
			if err == nil {
				usages[i] = usage
			}
		})
	}
	group.Wait()

	type total struct {
		sum   float64
		count int
	}
	order := []WtPoolLevelBucket{}
	totals := map[string]*total{}
	for _, usage := range usages {
		if usage == nil {
			continue
		}
		for _, usageGroup := range usage.Groups {
			for _, bucket := range usageGroup.Buckets {
				key := bucket.Name + "|" + bucket.Window
				if _, seen := totals[key]; !seen {
					totals[key] = &total{}
					order = append(order, WtPoolLevelBucket{Name: bucket.Name, Window: bucket.Window})
				}
				totals[key].sum += bucket.RemainingFraction
				totals[key].count++
			}
		}
	}
	for i := range order {
		sum := totals[order[i].Name+"|"+order[i].Window]
		order[i].AverageRemainingFraction = sum.sum / float64(sum.count)
	}
	wtPoolLevelMu.Lock()
	wtPoolLevel = WtPoolLevel{FetchedAt: now.Unix(), Accounts: len(credentials), Healthy: healthy, Buckets: order}
	wtPoolLevelMu.Unlock()
}
