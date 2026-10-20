package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// wind-tide: the fork's routes under /api/wt (wind-tide/API.md §4, §5,
// §7–§9, §14 and §15). Every dashboard route is also declared in
// middleware/access_token_routes.go.
func setWtRoutes(apiRouter *gin.RouterGroup) {
	wtRoute := apiRouter.Group("/wt")
	wtRoute.GET("/public-stats", controller.WtGetPublicStats)
	wtRoute.GET("/avatar/:discord_id", controller.WtGetAvatar)
	wtRoute.POST("/emergency-login", middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.AnonymousRequestBodyLimit(), controller.WtEmergencyLogin)
	wtRoute.GET("/leaderboard", middleware.DisableCache(), middleware.UserAuth(), controller.WtGetLeaderboard)

	ownerRoute := wtRoute.Group("")
	ownerRoute.Use(middleware.DisableCache(), middleware.RootAuth())
	{
		ownerRoute.GET("/allowlist", controller.WtListAllowlist)
		ownerRoute.POST("/allowlist", controller.WtAddAllowlistEntry)
		ownerRoute.PUT("/allowlist/:id", controller.WtUpdateAllowlistNote)
		ownerRoute.POST("/allowlist/:id/remove", controller.WtRemoveAllowlistEntry)
		ownerRoute.POST("/allowlist/:id/restore", controller.WtRestoreAllowlistEntry)
		ownerRoute.GET("/entry-requests", controller.WtListEntryRequests)
		ownerRoute.GET("/entry-requests/pending-count", controller.WtCountPendingEntryRequests)
		ownerRoute.POST("/entry-requests/:id/approve", controller.WtApproveEntryRequest)
		ownerRoute.POST("/entry-requests/:id/reject", controller.WtRejectEntryRequest)
	}

	// wind-tide: donated Antigravity accounts (wind-tide/API.md §14). Members
	// only, browser sessions only (middleware/access_token_routes.go); the
	// write routes share one per-member critical rate limit, tests another.
	donationRoute := wtRoute.Group("/donations")
	donationRoute.Use(middleware.DisableCache(), middleware.UserAuth())
	{
		donationRoute.POST("/start", middleware.UserCriticalRateLimit("wt-donation"), controller.WtStartDonation)
		donationRoute.POST("/callback", middleware.UserCriticalRateLimit("wt-donation"), controller.WtSubmitDonationCallback)
		donationRoute.GET("", controller.WtListDonations)
		donationRoute.GET("/:id", controller.WtGetDonation)
		donationRoute.GET("/:id/usage", controller.WtGetDonationUsage)
		donationRoute.POST("/:id/test", middleware.UserCriticalRateLimit("wt-donation-test"), controller.WtTestDonation)
		donationRoute.POST("/:id/remove", middleware.UserCriticalRateLimit("wt-donation"), controller.WtRemoveDonation)
		// wind-tide: donated AI Studio accounts (wind-tide/API.md §15) share
		// the list, the single-row routes and the write limit.
		donationRoute.POST("/aistudio", middleware.UserCriticalRateLimit("wt-donation"), controller.WtDonateAistudio)
		// wind-tide: the same Cookie donated to the BUILD pool (§15).
		donationRoute.POST("/build", middleware.UserCriticalRateLimit("wt-donation"), controller.WtDonateBuild)
	}

	// wind-tide: the pool page (wind-tide/API.md §15). Members only, browser
	// sessions only; giving and taking share the donation write limit.
	poolRoute := wtRoute.Group("/pool")
	poolRoute.Use(middleware.DisableCache(), middleware.UserAuth())
	{
		poolRoute.GET("", controller.WtGetPool)
		poolRoute.GET("/events", controller.WtListPoolEvents)
		poolRoute.POST("/ais/give", middleware.UserCriticalRateLimit("wt-donation"), controller.WtGiveAisCalls)
		poolRoute.POST("/ais/take", middleware.UserCriticalRateLimit("wt-donation"), controller.WtTakeAisCalls)
	}
}
