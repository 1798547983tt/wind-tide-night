package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// wind-tide: the fork's routes under /api/wt (wind-tide/API.md §4, §5 and
// §7–§9). Every dashboard route is also declared in
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
}
