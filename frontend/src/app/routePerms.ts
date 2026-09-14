/** 与后端 models/seed.go 权限码对齐；任一满足即可进入对应路由。 */
export const ROUTE_PERMS: Record<string, string[]> = {
  '/clusters': ['cluster:list', 'cluster:create', 'cluster:update', 'cluster:delete', 'cluster:ping'],
  '/clusters/list': ['cluster:list'],
  '/clusters/import': ['cluster:create'],
  '/resources': ['resource:list', 'resource:get', 'resource:create', 'resource:update', 'resource:delete'],
  '/resources/list': ['resource:list'],
  '/resources/edit': ['resource:get'],
  '/resources/quota': ['resource:list'],
  '/backups': ['backup:list', 'backup:create', 'backup:restore', 'backup:delete'],
  '/backups/list': ['backup:list'],
  '/backups/create': ['backup:create'],
  '/rbac': ['user:manage', 'role:manage'],
  '/rbac/users': ['user:manage'],
  '/rbac/roles': ['role:manage'],
  '/audit': ['audit:list'],
  '/audit/list': ['audit:list'],
  '/helm': ['helm:view', 'helm:install', 'helm:uninstall', 'helm:rollback'],
  '/helm/list': ['helm:view'],
  '/helm/repos': ['helm:view'],
  '/pods/logs': ['resource:get', 'resource:list'],
  '/ops/compare': ['resource:get'],
};

export function safeRedirectPath(raw: string | null | undefined): string {
  if (!raw) {
    return '/dashboard';
  }
  if (!raw.startsWith('/') || raw.startsWith('//') || raw.includes('://')) {
    return '/dashboard';
  }
  return raw;
}
