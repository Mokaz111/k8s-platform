import type { KubernetesResource } from '@/app/services/resource';

export interface WorkloadStatusView {
  text: string;
  color: string;
  extra?: string;
}

function num(v: unknown): number | undefined {
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  if (typeof v === 'string' && v !== '' && !Number.isNaN(Number(v))) return Number(v);
  return undefined;
}

function asRecord(v: unknown): Record<string, unknown> | undefined {
  if (v && typeof v === 'object' && !Array.isArray(v)) return v as Record<string, unknown>;
  return undefined;
}

const POD_PHASE_COLOR: Record<string, string> = {
  Pending: 'warning',
  Running: 'success',
  Succeeded: 'blue',
  Failed: 'error',
  Unknown: 'default',
};

function podWaitingReason(status: Record<string, unknown>): string | undefined {
  const containers = status.containerStatuses as
    | { ready?: boolean; state?: Record<string, unknown> }[]
    | undefined;
  const waiting = containers?.find((c) => c.state && 'waiting' in c.state);
  const waitingState = asRecord(waiting?.state?.waiting);
  if (typeof waitingState?.reason === 'string' && waitingState.reason) {
    return waitingState.reason;
  }
  const initContainers = status.initContainerStatuses as
    | { state?: Record<string, unknown> }[]
    | undefined;
  const initWaiting = initContainers?.find((c) => c.state && 'waiting' in c.state);
  const initState = asRecord(initWaiting?.state?.waiting);
  if (typeof initState?.reason === 'string' && initState.reason) {
    return `Init: ${initState.reason}`;
  }
  const conds = status.conditions as { type?: string; status?: string; reason?: string }[] | undefined;
  const scheduled = conds?.find((c) => c.type === 'PodScheduled');
  if (scheduled?.status === 'False' && scheduled.reason) {
    return scheduled.reason;
  }
  if (typeof status.reason === 'string' && status.reason) {
    return status.reason;
  }
  return undefined;
}

/** 从 K8s 资源 status 提取列表展示用状态（Pod Pending、Deployment 就绪副本等）。 */
export function getWorkloadStatus(kind: string, resource: KubernetesResource): WorkloadStatusView {
  const status = asRecord(resource.status) || {};
  const spec = asRecord(resource.spec) || {};

  switch (kind) {
    case 'Pod': {
      const phase = typeof status.phase === 'string' && status.phase ? status.phase : 'Unknown';
      const containers = status.containerStatuses as { ready?: boolean }[] | undefined;
      const total = containers?.length ?? 0;
      const ready = containers?.filter((c) => c.ready).length ?? 0;
      const reason = podWaitingReason(status);
      let extra = total ? `${ready}/${total} Ready` : undefined;
      if (phase !== 'Running' && reason) extra = reason;
      return { text: phase, color: POD_PHASE_COLOR[phase] || 'default', extra };
    }
    case 'Deployment':
    case 'StatefulSet':
    case 'ReplicaSet': {
      const desired = num(status.replicas) ?? num(spec.replicas) ?? 0;
      const ready = num(status.readyReplicas) ?? 0;
      const available = num(status.availableReplicas);
      let color = 'default';
      if (desired === 0) color = 'default';
      else if (ready === desired) color = 'success';
      else if (ready === 0) color = 'error';
      else color = 'warning';
      return {
        text: `${ready}/${desired}`,
        color,
        extra: available != null ? `可用 ${available}` : undefined,
      };
    }
    case 'DaemonSet': {
      const desired = num(status.desiredNumberScheduled) ?? 0;
      const ready = num(status.numberReady) ?? 0;
      const unavailable = num(status.numberUnavailable) ?? 0;
      let color = 'default';
      if (desired === 0) color = 'default';
      else if (ready === desired) color = 'success';
      else if (ready === 0) color = 'error';
      else color = 'warning';
      return {
        text: `${ready}/${desired}`,
        color,
        extra: unavailable ? `不可用 ${unavailable}` : undefined,
      };
    }
    case 'Job': {
      const succeeded = num(status.succeeded) ?? 0;
      const failed = num(status.failed) ?? 0;
      const active = num(status.active) ?? 0;
      const completions = num(spec.completions) ?? 1;
      if (failed > 0 && succeeded < completions) {
        return { text: 'Failed', color: 'error', extra: `失败 ${failed}` };
      }
      if (succeeded >= completions) {
        return { text: 'Complete', color: 'success', extra: `${succeeded}/${completions}` };
      }
      if (active > 0) {
        return { text: 'Running', color: 'processing', extra: `进行中 ${active}` };
      }
      return { text: 'Pending', color: 'warning' };
    }
    case 'CronJob': {
      if (spec.suspend) return { text: 'Suspended', color: 'default' };
      const active = status.active;
      const last = typeof status.lastScheduleTime === 'string' ? status.lastScheduleTime : undefined;
      if (Array.isArray(active) && active.length > 0) {
        return { text: 'Active', color: 'processing', extra: `${active.length} 个任务` };
      }
      return { text: 'Idle', color: 'success', extra: last ? `上次调度 ${last}` : undefined };
    }
    default: {
      const phase = status.phase;
      if (typeof phase === 'string' && phase) {
        const color = /bound|ready|available|active|running|complete/i.test(phase)
          ? 'success'
          : /pending|wait|progress/i.test(phase)
            ? 'warning'
            : /fail|error|lost|term|lost/i.test(phase)
              ? 'error'
              : 'default';
        return { text: phase, color };
      }
      return { text: '-', color: 'default' };
    }
  }
}
