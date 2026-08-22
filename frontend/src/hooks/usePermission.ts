import { useAppSelector } from '@/app/store';

/**
 * usePermission 提供前端权限点判断。
 *
 * 后端 /auth/me 返回 perms（权限点 code 列表）与 is_platform_admin。
 * 平台管理员默认拥有全部权限；普通用户按 perms 列表判断。
 *
 * 用法：
 *   const { hasPerm, isPlatformAdmin } = usePermission();
 *   {hasPerm('cluster:create') && <Button>导入集群</Button>}
 */
export const usePermission = () => {
  const currentUser = useAppSelector((s) => s.user.currentUser);
  const perms = currentUser?.perms || [];
  const isPlatformAdmin = currentUser?.is_platform_admin || false;

  // 是否拥有指定权限点
  const hasPerm = (code: string) => isPlatformAdmin || perms.includes(code);
  // 是否拥有任一权限点
  const hasAnyPerm = (codes: string[]) => isPlatformAdmin || codes.some((c) => perms.includes(c));
  // 是否同时拥有全部权限点
  const hasAllPerms = (codes: string[]) => isPlatformAdmin || codes.every((c) => perms.includes(c));

  return { perms, isPlatformAdmin, hasPerm, hasAnyPerm, hasAllPerms };
};
