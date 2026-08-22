package api

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/handler"
)

func RegisterResourceRoutes(rg *gin.RouterGroup, h *handler.ResourceHandler) {
	resources := rg.Group("/clusters/:code/resources")
	{
		resources.GET("/:apiVersion/:kind",
			RequirePermission("resource:list"),
			h.ListResources)

		resources.GET("/:apiVersion/:kind/:namespace/:name",
			RequirePermission("resource:get"),
			h.GetResource)

		resources.POST("/:apiVersion/:kind",
			RequirePermission("resource:create"),
			h.CreateResource)

		resources.PUT("/:apiVersion/:kind/:namespace/:name",
			RequirePermission("resource:update"),
			h.UpdateResource)

		resources.DELETE("/:apiVersion/:kind/:namespace/:name",
			RequirePermission("resource:delete"),
			h.DeleteResource)
	}
}
