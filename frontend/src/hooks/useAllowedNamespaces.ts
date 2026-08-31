import { useMemo } from 'react';
import { useListNamespacesQuery } from '@/app/services/resource';
import { useAppSelector } from '@/app/store';

/**
 * 当前用户在指定集群下可访问的命名空间。
 * 全集群/平台权限走 Namespace List API；仅 namespace 授权则用 /auth/me 的 roles，避免 403。
 */
export function useAllowedNamespaces(clusterCode: string | null | undefined): {
  namespaces: string[];
  isFullCluster: boolean;
  isLoading: boolean;
} {
  const currentUser = useAppSelector((s) => s.user.currentUser);

  const scope = useMemo(() => {
    if (!clusterCode) {
      return { isFullCluster: false, fromRoles: [] as string[] };
    }
    if (currentUser?.is_platform_admin) {
      return { isFullCluster: true, fromRoles: [] as string[] };
    }
    const fromRoles = new Set<string>();
    let isFullCluster = false;
    for (const role of currentUser?.roles || []) {
      if (role.scope_type === 'platform') {
        isFullCluster = true;
        break;
      }
      if (role.scope_type === 'cluster' && role.cluster_code === clusterCode) {
        isFullCluster = true;
        break;
      }
      if (
        role.scope_type === 'namespace' &&
        role.cluster_code === clusterCode &&
        role.namespace
      ) {
        fromRoles.add(role.namespace);
      }
    }
    return { isFullCluster, fromRoles: [...fromRoles].sort() };
  }, [clusterCode, currentUser]);

  const { data, isFetching } = useListNamespacesQuery(clusterCode || '', {
    skip: !clusterCode || !scope.isFullCluster,
    refetchOnMountOrArgChange: true,
  });

  if (scope.isFullCluster) {
    return {
      namespaces: data || [],
      isFullCluster: true,
      isLoading: isFetching,
    };
  }
  return {
    namespaces: scope.fromRoles,
    isFullCluster: false,
    isLoading: false,
  };
}
