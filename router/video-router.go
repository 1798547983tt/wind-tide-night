package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	videoSharedRouter := router.Group("/v1")
	videoSharedRouter.Use(middleware.RouteTag("relay"))
	videoSharedRouter.Use(middleware.TokenAuth())
	videoSharedRouter.Use(middleware.SystemPerformanceCheck())
	videoSharedRouter.POST(
		"/video/generations",
		middleware.PinTaskPluginEndpoint(),
		// wind-tide: every video generation counts against the model rate
		// limit, not only plugin endpoints (wind-tide/API.md §10).
		middleware.ModelRequestRateLimit(),
		middleware.PrepareTaskPluginEndpoint(),
		middleware.Distribute(),
		func(c *gin.Context) {
			controller.RelayTaskPluginEndpoint(c, controller.RelayTask)
		},
	)

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth())
	// wind-tide: a remix is a new generation and counts against the model
	// rate limit (wind-tide/API.md §10).
	videoRemixRouter := videoV1Router.Group("", middleware.ModelRequestRateLimit(), middleware.Distribute())
	videoV1Router.Use(middleware.Distribute())
	{
		videoV1Router.GET("/video/generations/:task_id", controller.RelayTaskFetch)
		videoRemixRouter.POST("/videos/:video_id/remix", controller.RelayTask)
	}
}
