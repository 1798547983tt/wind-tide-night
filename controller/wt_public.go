package controller

import (
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// wind-tide: leaderboards, home page numbers and cached avatars
// (wind-tide/API.md §7, §8 and §9).

// wtStatsCacheSeconds is how long a leaderboard or the public numbers are
// served from memory.
const wtStatsCacheSeconds = 60

// wtAvatarMaxAge caps how long browsers keep an avatar; never past the
// expiry of its address.
const wtAvatarMaxAge = 86400

var (
	wtLeaderboardMu    sync.Mutex
	wtLeaderboards     = map[string]*model.WtLeaderboard{}
	wtPublicStatsMu    sync.Mutex
	wtPublicStatsCache *model.WtPublicStats
)

// WtGetLeaderboard serves a leaderboard to signed-in members. The calls board
// takes period today (default), week or all; the balance board has no period.
func WtGetLeaderboard(c *gin.Context) {
	board := c.DefaultQuery("board", model.WtBoardCalls)
	period := c.Query("period")
	switch {
	case !model.WtKnownBoard(board):
		common.ApiErrorT(c, "Invalid parameters")
		return
	case model.WtBoardHasPeriod(board):
		if period == "" {
			period = model.WtPeriodToday
		}
		if period != model.WtPeriodToday && period != model.WtPeriodWeek && period != model.WtPeriodAll {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
	default:
		period = ""
	}

	now := time.Now()
	key := board + ":" + period
	wtLeaderboardMu.Lock()
	leaderboard := wtLeaderboards[key]
	if leaderboard == nil || now.Unix()-leaderboard.UpdatedAt >= wtStatsCacheSeconds {
		built, err := model.BuildWtLeaderboard(board, period, now)
		if err != nil {
			wtLeaderboardMu.Unlock()
			common.ApiError(c, err)
			return
		}
		wtLeaderboards[key] = built
		leaderboard = built
	}
	wtLeaderboardMu.Unlock()

	common.ApiSuccess(c, gin.H{
		"board":      leaderboard.Board,
		"period":     leaderboard.Period,
		"items":      leaderboard.Top(),
		"me":         leaderboard.Me(c.GetInt("id")),
		"updated_at": leaderboard.UpdatedAt,
	})
}

// WtGetPublicStats serves the home page numbers to everyone.
func WtGetPublicStats(c *gin.Context) {
	now := time.Now()
	wtPublicStatsMu.Lock()
	stats := wtPublicStatsCache
	if stats == nil || now.Unix()-stats.UpdatedAt >= wtStatsCacheSeconds {
		built, err := model.GetWtPublicStats(now)
		if err != nil {
			wtPublicStatsMu.Unlock()
			common.ApiError(c, err)
			return
		}
		wtPublicStatsCache = built
		stats = built
	}
	wtPublicStatsMu.Unlock()
	common.ApiSuccess(c, stats)
}

// WtGetAvatar serves the cached avatar of a Discord ID that belongs to an
// account or an entry request, at a signed address from model.WtAvatarURL.
// Unsigned or expired addresses, unknown IDs, accounts without a custom avatar
// and avatars that are not cached yet all answer the same 404; a missing file
// is fetched in the background for the next request.
func WtGetAvatar(c *gin.Context) {
	discordID := c.Param("discord_id")
	ttl, signed := model.WtAvatarURLTTL(discordID, c.Query("exp"), c.Query("sig"), model.WtNow())
	if !signed {
		c.Status(http.StatusNotFound)
		return
	}
	hash, known, err := model.WtKnownAvatarHash(discordID)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	if !known || hash == "" {
		c.Status(http.StatusNotFound)
		return
	}
	path := service.WtAvatarPath(discordID, hash)
	if _, err := os.Stat(path); err != nil {
		service.WtRefreshAvatarAsync(discordID, hash)
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age="+strconv.FormatInt(min(ttl, wtAvatarMaxAge), 10))
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(path)
}
