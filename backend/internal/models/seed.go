package models

import (
	"fmt"

	"github.com/k8s-platform/console/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MVP 阶段预置权限点清单（与设计文档 9.2 节保持一致）
var seedPermissionsData = []SysPermission{
	{Code: "auth:login", Module: "auth", Action: "login", Name: "登录登出", Description: "用户登录、登出、刷新令牌"},
	{Code: "cluster:list", Module: "cluster", Action: "list", Name: "查看集群列表", Description: "查看集群列表与详情"},
	{Code: "cluster:create", Module: "cluster", Action: "create", Name: "导入集群", Description: "导入新集群、上传 kubeconfig"},
	{Code: "cluster:update", Module: "cluster", Action: "update", Name: "更新集群信息", Description: "修改集群名称、描述、重新上传证书"},
	{Code: "cluster:delete", Module: "cluster", Action: "delete", Name: "删除集群", Description: "从平台移除集群纳管"},
	{Code: "cluster:ping", Module: "cluster", Action: "ping", Name: "集群连通性检测", Description: "主动 ping 集群，检查健康状态"},
	{Code: "resource:list", Module: "resource", Action: "list", Name: "查看资源列表", Description: "查看各类 K8S 资源列表（含命名空间筛选）"},
	{Code: "resource:get", Module: "resource", Action: "get", Name: "查看资源详情", Description: "查看单个资源 YAML/详情"},
	{Code: "resource:create", Module: "resource", Action: "create", Name: "新建资源", Description: "通过 YAML 新建 K8S 资源"},
	{Code: "resource:update", Module: "resource", Action: "update", Name: "编辑资源", Description: "修改并保存资源 YAML"},
	{Code: "resource:delete", Module: "resource", Action: "delete", Name: "删除资源", Description: "删除 K8S 资源"},
	{Code: "version:list", Module: "version", Action: "list", Name: "查看版本历史", Description: "查看资源的版本快照列表"},
	{Code: "version:diff", Module: "version", Action: "diff", Name: "版本对比", Description: "对比两个版本 YAML 差异"},
	{Code: "version:rollback", Module: "version", Action: "rollback", Name: "版本回滚", Description: "将资源回滚至选定历史版本"},
	{Code: "backup:list", Module: "backup", Action: "list", Name: "查看备份列表", Description: "查看备份任务记录与备份文件"},
	{Code: "backup:create", Module: "backup", Action: "create", Name: "新建备份", Description: "对单个资源或整个命名空间发起备份"},
	{Code: "backup:restore", Module: "backup", Action: "restore", Name: "从备份恢复", Description: "将备份 YAML 应用至集群"},
	{Code: "backup:delete", Module: "backup", Action: "delete", Name: "删除备份", Description: "删除备份任务及备份文件"},
	{Code: "helm:view", Module: "helm", Action: "view", Name: "查看 Helm Release", Description: "查看 Helm release 列表与修订历史"},
	{Code: "helm:install", Module: "helm", Action: "install", Name: "安装/升级 Helm", Description: "通过 Helm 安装或升级应用"},
	{Code: "helm:uninstall", Module: "helm", Action: "uninstall", Name: "卸载 Helm", Description: "卸载 Helm release"},
	{Code: "helm:rollback", Module: "helm", Action: "rollback", Name: "回滚 Helm", Description: "回滚 Helm release 到指定修订版本"},
	{Code: "plugin:list", Module: "plugin", Action: "list", Name: "查看插件中心", Description: "浏览已安装插件与能力"},
	{Code: "plugin:enable", Module: "plugin", Action: "enable", Name: "启用/禁用插件", Description: "启用或禁用插件"},
	{Code: "system:config", Module: "system", Action: "config", Name: "系统设置", Description: "修改系统配置、存储后端、通知通道等"},
	{Code: "audit:list", Module: "audit", Action: "list", Name: "查询审计日志", Description: "查看与筛选操作审计记录"},
	{Code: "user:manage", Module: "user", Action: "manage", Name: "用户管理", Description: "新增/禁用/编辑平台用户"},
	{Code: "role:manage", Module: "role", Action: "manage", Name: "角色权限管理", Description: "创建/编辑角色、分配权限点、绑定用户-角色"},
}

// 内置角色：platform-admin、cluster-admin、namespace-developer、platform-viewer
type seedRoleDef struct {
	Role        SysRole
	Perms       []string // 匹配 SysPermission.Code，支持 "module:*" 通配
}

var seedRolesData = []seedRoleDef{
	{
		Role:  SysRole{Code: "platform-admin", Name: "平台管理员", Description: "全平台最高权限（内置不可删除）", Builtin: 1},
		Perms: []string{"*:*"},
	},
	{
		Role: SysRole{Code: "cluster-admin", Name: "集群管理员", Description: "负责指定集群的完整运维管理", Builtin: 1},
		Perms: []string{
			"cluster:list", "cluster:ping",
			"resource:*", "version:*", "backup:*",
			"plugin:list",
		},
	},
	{
		Role: SysRole{Code: "namespace-developer", Name: "命名空间开发者", Description: "对指定命名空间下资源有开发权限", Builtin: 1},
		Perms: []string{
			"cluster:list", "cluster:ping",
			"resource:list", "resource:get", "resource:create", "resource:update",
			"version:list", "version:diff",
			"backup:list", "backup:create", "backup:restore",
		},
	},
	{
		Role: SysRole{Code: "platform-viewer", Name: "平台只读用户", Description: "仅可查看全平台资源，无修改操作", Builtin: 1},
		Perms: []string{
			"cluster:list", "cluster:ping",
			"resource:list", "resource:get",
			"version:list", "version:diff",
			"backup:list",
			"plugin:list",
		},
	},
}

func seedPermissions(db *gorm.DB) error {
	// Upsert 按 code，保留已存在的不覆盖
	for i := range seedPermissionsData {
		p := seedPermissionsData[i]
		res := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "code"}},
			DoNothing: true,
		}).Create(&p)
		if res.Error != nil {
			return res.Error
		}
	}
	return nil
}

func seedRoles(db *gorm.DB) error {
	// 加载所有权限点
	var allPerms []SysPermission
	if err := db.Find(&allPerms).Error; err != nil {
		return err
	}
	permMap := make(map[string]uint64, len(allPerms))
	for _, p := range allPerms {
		permMap[p.Code] = p.ID
	}

	for _, sd := range seedRolesData {
		// 创建或获取角色（Upsert）
		role := sd.Role
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "code"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"name":        role.Name,
				"description": role.Description,
				"builtin":     role.Builtin,
				"status":      1,
			}),
		}).Create(&role).Error; err != nil {
			return err
		}
		if role.ID == 0 {
			// Create 走 DoUpdates 不返回 ID，重新查
			var find SysRole
			if err := db.Where("code=?", role.Code).First(&find).Error; err != nil {
				return err
			}
			role.ID = find.ID
		}

		// 匹配通配符，计算应该绑定的权限点集合
		bindCodes := make(map[string]struct{})
		for _, p := range sd.Perms {
			if p == "*:*" {
				for _, pp := range allPerms {
					bindCodes[pp.Code] = struct{}{}
				}
				continue
			}
			// module:*
			if len(p) > 2 && p[len(p)-2:] == ":*" {
				mod := p[:len(p)-2]
				for _, pp := range allPerms {
					if pp.Module == mod {
						bindCodes[pp.Code] = struct{}{}
					}
				}
				continue
			}
			if _, exists := permMap[p]; exists {
				bindCodes[p] = struct{}{}
			}
		}

		// 先清空旧绑定，再批量插入（简单可靠）
		if err := db.Where("role_id=?", role.ID).Delete(&SysRolePermission{}).Error; err != nil {
			return err
		}
		var binds []SysRolePermission
		for code := range bindCodes {
			binds = append(binds, SysRolePermission{RoleID: role.ID, PermissionCode: code})
		}
		if len(binds) > 0 {
			if err := db.Create(&binds).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// seedAdminUser 创建默认管理员：admin / admin123（仅当不存在时）
// 绑定 platform-admin 角色，scope=platform
func seedAdminUser(db *gorm.DB, log *logger.Logger) error {
	var count int64
	if err := db.Model(&SysUser{}).Where("username=?", "admin").Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	admin := &SysUser{
		Username:    "admin",
		DisplayName: "平台超级管理员",
		Email:       "admin@example.local",
		AuthSource:  "local",
		Status:      UserStatusEnabled,
	}
	if err := admin.SetPassword("admin123", 10); err != nil {
		return err
	}
	if err := db.Create(admin).Error; err != nil {
		return err
	}

	// 查询 platform-admin 角色
	var role SysRole
	if err := db.Where("code=?", "platform-admin").First(&role).Error; err != nil {
		return fmt.Errorf("find platform-admin role: %w", err)
	}

	// 绑定
	if err := db.Create(&SysUserRole{
		UserID:    admin.ID,
		RoleID:    role.ID,
		ScopeType: ScopePlatform,
	}).Error; err != nil {
		return err
	}

	log.Warnf("⚠️  Created default admin user: admin / admin123. PLEASE CHANGE PASSWORD ON FIRST LOGIN!")
	return nil
}
