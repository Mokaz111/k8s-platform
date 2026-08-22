import React, { useEffect, useMemo, useState } from 'react';
import { Card, Form, Select, Space, Tag, Typography } from 'antd';
import { PageContainer } from '@ant-design/pro-components';
import { ArrowLeftOutlined, FileTextOutlined } from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useListClustersQuery } from '@/app/services/cluster';
import {
  KubernetesResource,
  useListNamespacesQuery,
  useListResourcesQuery,
} from '@/app/services/resource';
import { PodLogsViewer } from '@/components/ws';

const { Text } = Typography;

// 从 Pod spec 中提取容器名
function extractContainers(pod: KubernetesResource): string[] {
  const containers = (pod.spec as { containers?: { name?: string }[] } | undefined)?.containers;
  if (!Array.isArray(containers)) return [];
  return containers
    .map((c) => c?.name)
    .filter((n): n is string => typeof n === 'string' && n.length > 0);
}

const PodLogsPage: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const initialClusterCode = searchParams.get('code') || '';
  const initialNamespace = searchParams.get('namespace') || '';
  const initialPodName = searchParams.get('pod') || '';
  const initialContainer = searchParams.get('container') || undefined;

  const [clusterCode, setClusterCode] = useState<string>(initialClusterCode);
  const [namespace, setNamespace] = useState<string>(initialNamespace);
  const [podName, setPodName] = useState<string>(initialPodName);
  const [containerName, setContainerName] = useState<string | undefined>(
    initialContainer,
  );

  const { data: clusterData } = useListClustersQuery(undefined, {
    refetchOnMountOrArgChange: true,
  });
  const clusters = clusterData?.list || [];

  const { data: namespaces } = useListNamespacesQuery(clusterCode || '', {
    skip: !clusterCode,
    refetchOnMountOrArgChange: true,
  });

  // 加载 Pod 列表
  const { data: podsData, isFetching: loadingPods } = useListResourcesQuery(
    {
      code: clusterCode,
      apiVersion: 'v1',
      kind: 'Pod',
      namespace,
      size: 200,
    },
    {
      skip: !clusterCode || !namespace,
      refetchOnMountOrArgChange: true,
    },
  );
  const podItems = podsData?.items || [];

  // 同步 URL
  useEffect(() => {
    const next = new URLSearchParams();
    if (clusterCode) next.set('code', clusterCode);
    if (namespace) next.set('namespace', namespace);
    if (podName) next.set('pod', podName);
    if (containerName) next.set('container', containerName);
    setSearchParams(next, { replace: true });
  }, [clusterCode, namespace, podName, containerName, setSearchParams]);

  const currentPod = useMemo(
    () => podItems.find((p) => p.metadata.name === podName),
    [podItems, podName],
  );

  const containerOptions = useMemo(() => {
    if (!currentPod) return [];
    return extractContainers(currentPod).map((c) => ({ value: c, label: c }));
  }, [currentPod]);

  return (
    <PageContainer
      onBack={() => navigate(-1)}
      backIcon={<ArrowLeftOutlined />}
      title={
        <Space>
          <FileTextOutlined />
          <span>Pod 日志查看</span>
        </Space>
      }
    >
      <Card bordered={false} style={{ marginBottom: 16 }}>
        <Form layout="inline">
          <Form.Item label="集群">
            <Select
              style={{ width: 220 }}
              showSearch
              optionFilterProp="label"
              placeholder="选择集群"
              value={clusterCode || undefined}
              onChange={(v) => {
                setClusterCode(v || '');
                setNamespace('');
                setPodName('');
                setContainerName(undefined);
              }}
              options={clusters.map((c) => ({
                value: c.code,
                label: `${c.name} (${c.code})`,
              }))}
              allowClear
            />
          </Form.Item>
          <Form.Item label="命名空间">
            <Select
              style={{ width: 200 }}
              showSearch
              placeholder="选择命名空间"
              value={namespace || undefined}
              disabled={!clusterCode}
              onChange={(v) => {
                setNamespace(v || '');
                setPodName('');
                setContainerName(undefined);
              }}
              options={(namespaces || []).map((n) => ({ value: n, label: n }))}
              allowClear
            />
          </Form.Item>
          <Form.Item label="Pod">
            <Select
              style={{ width: 280 }}
              showSearch
              optionFilterProp="label"
              placeholder="选择 Pod"
              loading={loadingPods}
              value={podName || undefined}
              disabled={!namespace}
              onChange={(v) => {
                setPodName(v || '');
                setContainerName(undefined);
              }}
              options={podItems.map((p) => ({
                value: p.metadata.name,
                label: p.metadata.name,
              }))}
              allowClear
            />
          </Form.Item>
          <Form.Item label="容器">
            <Select
              style={{ width: 200 }}
              placeholder="（可选）选择容器"
              value={containerName}
              disabled={!podName || containerOptions.length === 0}
              onChange={(v) => setContainerName(v || undefined)}
              options={containerOptions}
              allowClear
            />
          </Form.Item>
        </Form>
      </Card>

      <Card bordered={false} bodyStyle={{ padding: 0 }}>
        {clusterCode && namespace && podName ? (
          <PodLogsViewer
            clusterCode={clusterCode}
            namespace={namespace}
            podName={podName}
            containerName={containerName}
            height={640}
            follow
            tailLines={500}
          />
        ) : (
          <div
            style={{
              height: 320,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: 'rgba(0,0,0,0.45)',
            }}
          >
            <Space direction="vertical" align="center">
              <Tag color="default">提示</Tag>
              <Text type="secondary">
                请先选择 集群 / 命名空间 / Pod，将自动开始实时日志流推送
              </Text>
            </Space>
          </div>
        )}
      </Card>
    </PageContainer>
  );
};

export default PodLogsPage;
