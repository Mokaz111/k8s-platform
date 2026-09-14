export type ResourceTab = 'workload' | 'network' | 'config' | 'storage';

export interface KindItem {
  label: string;
  value: string;
  apiVersion: string;
  clusterScoped?: boolean;
}

export interface KindGroup {
  key: ResourceTab;
  label: string;
  kinds: KindItem[];
}

export const KIND_GROUPS: KindGroup[] = [
  {
    key: 'workload',
    label: '工作负载',
    kinds: [
      { label: 'Deployment', value: 'Deployment', apiVersion: 'apps/v1' },
      { label: 'StatefulSet', value: 'StatefulSet', apiVersion: 'apps/v1' },
      { label: 'DaemonSet', value: 'DaemonSet', apiVersion: 'apps/v1' },
      { label: 'Job', value: 'Job', apiVersion: 'batch/v1' },
      { label: 'CronJob', value: 'CronJob', apiVersion: 'batch/v1' },
      { label: 'Pod', value: 'Pod', apiVersion: 'v1' },
    ],
  },
  {
    key: 'network',
    label: '网络',
    kinds: [
      { label: 'Service', value: 'Service', apiVersion: 'v1' },
      { label: 'Ingress', value: 'Ingress', apiVersion: 'networking.k8s.io/v1' },
      { label: 'NetworkPolicy', value: 'NetworkPolicy', apiVersion: 'networking.k8s.io/v1' },
      { label: 'Endpoints', value: 'Endpoints', apiVersion: 'v1' },
    ],
  },
  {
    key: 'config',
    label: '配置',
    kinds: [
      { label: 'ConfigMap', value: 'ConfigMap', apiVersion: 'v1' },
      { label: 'Secret', value: 'Secret', apiVersion: 'v1' },
      { label: 'Namespace', value: 'Namespace', apiVersion: 'v1', clusterScoped: true },
      { label: 'ServiceAccount', value: 'ServiceAccount', apiVersion: 'v1' },
      { label: 'ResourceQuota', value: 'ResourceQuota', apiVersion: 'v1' },
      { label: 'LimitRange', value: 'LimitRange', apiVersion: 'v1' },
    ],
  },
  {
    key: 'storage',
    label: '存储',
    kinds: [
      { label: 'PersistentVolume', value: 'PersistentVolume', apiVersion: 'v1', clusterScoped: true },
      { label: 'PersistentVolumeClaim', value: 'PersistentVolumeClaim', apiVersion: 'v1' },
      { label: 'StorageClass', value: 'StorageClass', apiVersion: 'storage.k8s.io/v1', clusterScoped: true },
      { label: 'VolumeAttachment', value: 'VolumeAttachment', apiVersion: 'storage.k8s.io/v1', clusterScoped: true },
    ],
  },
];

export const ALL_KINDS_MAP: Record<string, { apiVersion: string; clusterScoped?: boolean }> = {};
KIND_GROUPS.forEach((g) =>
  g.kinds.forEach((k) => {
    ALL_KINDS_MAP[k.value] = { apiVersion: k.apiVersion, clusterScoped: k.clusterScoped };
  }),
);

export const ALL_KIND_OPTIONS = KIND_GROUPS.flatMap((g) =>
  g.kinds.map((k) => ({
    label: `${g.label} / ${k.label}`,
    value: k.value,
    apiVersion: k.apiVersion,
    clusterScoped: !!k.clusterScoped,
  })),
);
