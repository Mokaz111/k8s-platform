package api

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/handler"
	"github.com/k8s-platform/console/internal/api/middleware"
	"github.com/k8s-platform/console/internal/auth"
	"gorm.io/gorm"
)

type Handlers struct {
	Auth  *handler.AuthHandler
	User  *handler.UserHandler
	Role  *handler.RoleHandler
	Audit *handler.AuditHandler
}

func NewHandlers(authSvc *auth.Service, db *gorm.DB) *Handlers {
	return &Handlers{
		Auth:  handler.NewAuthHandler(authSvc, db),
		User:  handler.NewUserHandler(db, authSvc),
		Role:  handler.NewRoleHandler(db, authSvc),
		Audit: handler.NewAuditHandler(db),
	}
}

func RegisterAuthRoutes(r *gin.RouterGroup, authSvc *auth.Service, h *Handlers) {
	public := r.Group("/auth")
	{
		public.POST("/login", h.Auth.LoginHandler)
		public.POST("/refresh", h.Auth.RefreshHandler)
	}

	authorized := r.Group("")
	authorized.Use(middleware.AuthMiddleware(authSvc))
	{
		authGrp := authorized.Group("/auth")
		{
			authGrp.GET("/me", h.Auth.CurrentUserHandler)
			authGrp.POST("/logout", h.Auth.LogoutHandler)
		}
	}
}

func RegisterUserRoutes(r *gin.RouterGroup, authSvc *auth.Service, h *Handlers) {
	authorized := r.Group("")
	authorized.Use(middleware.AuthMiddleware(authSvc))
	authorized.Use(middleware.RBACMiddleware(authSvc))
	{
		users := authorized.Group("/users")
		{
			users.GET("", h.User.ListUsers)
			users.POST("", h.User.CreateUser)
			users.PUT("/:id", h.User.UpdateUser)
			users.PATCH("/:id/status", h.User.UpdateUserStatus)
			users.POST("/:id/reset-password", h.User.ResetPassword)
			users.GET("/:id/roles", h.User.ListUserRoles)
			users.POST("/:id/roles", h.User.BindUserRole)
			users.DELETE("/:id/roles", h.User.UnbindUserRole)
		}
	}
}

func RegisterRoleRoutes(r *gin.RouterGroup, authSvc *auth.Service, h *Handlers) {
	authorized := r.Group("")
	authorized.Use(middleware.AuthMiddleware(authSvc))
	authorized.Use(middleware.RBACMiddleware(authSvc))
	{
		roles := authorized.Group("/roles")
		{
			roles.GET("", h.Role.ListRoles)
			roles.POST("", h.Role.CreateRole)
			roles.PUT("/:id", h.Role.UpdateRole)
			roles.DELETE("/:id", h.Role.DeleteRole)
			roles.GET("/:id/permissions", h.Role.GetRolePermissions)
			roles.POST("/:id/permissions", h.Role.SetRolePermissions)
		}
		authorized.GET("/permissions", h.Role.ListPermissions)
	}
}

func RegisterAuditRoutes(r *gin.RouterGroup, authSvc *auth.Service, h *Handlers) {
	authorized := r.Group("")
	authorized.Use(middleware.AuthMiddleware(authSvc))
	authorized.Use(middleware.RBACMiddleware(authSvc))
	{
		authorized.GET("/audit-logs", h.Audit.ListAuditLogs)
	}
}
