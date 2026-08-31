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

  const hasPerm = (code: string) => {
    if (isPlatformAdmin || perms.includes('*:*') || perms.includes(code)) {
      return true;
    }
    const idx = code.indexOf(':');
    return idx > 0 && perms.includes(`${code.slice(0, idx)}:*`);
  };
  const hasAnyPerm = (codes: string[]) => isPlatformAdmin || codes.some((c) => hasPerm(c));
  const hasAllPerms = (codes: string[]) => isPlatformAdmin || codes.every((c) => hasPerm(c));

  return { perms, isPlatformAdmin, hasPerm, hasAnyPerm, hasAllPerms };
};
