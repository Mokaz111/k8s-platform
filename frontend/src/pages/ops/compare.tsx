import React, { useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  Row,
  Select,
  Space,
  Typography,
  message,
} from 'antd';
import { PageContainer } from '@ant-design/pro-components';
import { DiffOutlined, ReloadOutlined, SwapOutlined } from '@ant-design/icons';
import { useSearchParams } from 'react-router-dom';
import { useListClustersQuery } from '@/app/services/cluster';
import { useLazyGetResourceQuery } from '@/app/services/resource';
import { useAllowedNamespaces } from '@/hooks/useAllowedNamespaces';
import { YamlDiffEditor } from '@/components/YamlEditor';
import { ALL_KIND_OPTIONS, ALL_KINDS_MAP } from '@/constants/k8sKinds';
import { resourceToYaml } from '@/utils/yaml';

const { Text } = Typography;

interface CompareForm {
  leftCluster: string;
  rightCluster: string;
  leftNamespace: string;
  rightNamespace: string;
  kind: string;
  name: string;
}

const ClusterCompare: React.FC = () => {
  const [searchParams, setSearchParams] = useSearchParams();
  const [form] = Form.useForm<CompareForm>();
  const kindWatch = Form.useWatch('kind', form);
  const leftCluster = Form.useWatch('leftCluster', form);
  const rightCluster = Form.useWatch('rightCluster', form);

  const kindMeta = ALL_KINDS_MAP[kindWatch] || ALL_KIND_OPTIONS[0];
  const isClusterScoped = !!kindMeta?.clusterScoped;

  const { data: clusterData } = useListClustersQuery(undefined, {
    refetchOnMountOrArgChange: true,
  });
  const clusters = clusterData?.items || [];
  const clusterOptions = useMemo(
    () =>
      clusters.map((c) => ({
        label: c.name ? `${c.name} (${c.code})` : c.code,
        value: c.code,
      })),
    [clusters],
  );

  const { namespaces: leftNs, isFullCluster: leftFull } = useAllowedNamespaces(leftCluster);
  const { namespaces: rightNs, isFullCluster: rightFull } = useAllowedNamespaces(rightCluster);

  const leftNsOptions = useMemo(
    () => leftNs.map((n) => ({ label: n, value: n })),
    [leftNs],
  );
  const rightNsOptions = useMemo(
    () => rightNs.map((n) => ({ label: n, value: n })),
    [rightNs],
  );

  const [fetchResource] = useLazyGetResourceQuery();
  const [leftYaml, setLeftYaml] = useState('');
  const [rightYaml, setRightYaml] = useState('');
  const [leftError, setLeftError] = useState('');
  const [rightError, setRightError] = useState('');
  const [compared, setCompared] = useState(false);
  const [loading, setLoading] = useState(false);
  const [summary, setSummary] = useState('');

  useEffect(() => {
    form.setFieldsValue({
      leftCluster: searchParams.get('left') || clusters[0]?.code,
      rightCluster: searchParams.get('right') || clusters[1]?.code || clusters[0]?.code,
      kind: searchParams.get('kind') || 'Deployment',
      name: searchParams.get('name') || '',
      leftNamespace: searchParams.get('left_ns') || 'default',
      rightNamespace: searchParams.get('right_ns') || 'default',
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clusters.length]);

  useEffect(() => {
    if (!leftFull && leftNs.length > 0) {
      const cur = form.getFieldValue('leftNamespace');
      if (!leftNs.includes(cur)) form.setFieldValue('leftNamespace', leftNs[0]);
    }
  }, [leftFull, leftNs, form]);

  useEffect(() => {
    if (!rightFull && rightNs.length > 0) {
      const cur = form.getFieldValue('rightNamespace');
      if (!rightNs.includes(cur)) form.setFieldValue('rightNamespace', rightNs[0]);
    }
  }, [rightFull, rightNs, form]);

  const handleCompare = async () => {
    try {
      const values = await form.validateFields();
      if (values.leftCluster === values.rightCluster && !isClusterScoped) {
        if (values.leftNamespace === values.rightNamespace) {
          message.warning('请选择两个不同的集群，或同一集群下不同命名空间');
        }
      }
      const apiVersion = ALL_KINDS_MAP[values.kind]?.apiVersion;
      if (!apiVersion) {
        message.error('未知资源类型');
        return;
      }
      const next = new URLSearchParams();
      next.set('left', values.leftCluster);
      next.set('right', values.rightCluster);
      next.set('kind', values.kind);
      next.set('name', values.name);
      if (!isClusterScoped) {
        next.set('left_ns', values.leftNamespace);
        next.set('right_ns', values.rightNamespace);
      }
      setSearchParams(next, { replace: true });

      setLoading(true);
      setCompared(false);
      setLeftError('');
      setRightError('');
      setLeftYaml('');
      setRightYaml('');

      const leftNsVal = isClusterScoped ? '_' : values.leftNamespace;
      const rightNsVal = isClusterScoped ? '_' : values.rightNamespace;

      const [leftRes, rightRes] = await Promise.allSettled([
        fetchResource({
          code: values.leftCluster,
          apiVersion,
          kind: values.kind,
          namespace: leftNsVal,
          name: values.name.trim(),
        }).unwrap(),
        fetchResource({
          code: values.rightCluster,
          apiVersion,
          kind: values.kind,
          namespace: rightNsVal,
          name: values.name.trim(),
        }).unwrap(),
      ]);

      let leftText = '';
      let rightText = '';
      if (leftRes.status === 'fulfilled') {
        leftText = resourceToYaml(leftRes.value);
        setLeftYaml(leftText);
      } else {
        const msg =
          (leftRes.reason as { message?: string })?.message || '左侧集群加载失败';
        setLeftError(msg);
        setLeftYaml(`# ${msg}\n`);
      }
      if (rightRes.status === 'fulfilled') {
        rightText = resourceToYaml(rightRes.value);
        setRightYaml(rightText);
      } else {
        const msg =
          (rightRes.reason as { message?: string })?.message || '右侧集群加载失败';
        setRightError(msg);
        setRightYaml(`# ${msg}\n`);
      }

      const same = leftText !== '' && leftText === rightText;
      setSummary(
        same
          ? '两侧 YAML 完全一致'
          : leftRes.status === 'fulfilled' && rightRes.status === 'fulfilled'
            ? '两侧 YAML 存在差异'
            : '未能完整加载两侧配置',
      );
      setCompared(true);
    } catch {
      // form validation
    } finally {
      setLoading(false);
    }
  };

  const swapClusters = () => {
    const left = form.getFieldValue('leftCluster');
    const right = form.getFieldValue('rightCluster');
    const leftNsVal = form.getFieldValue('leftNamespace');
    const rightNsVal = form.getFieldValue('rightNamespace');
    form.setFieldsValue({
      leftCluster: right,
      rightCluster: left,
      leftNamespace: rightNsVal,
      rightNamespace: leftNsVal,
    });
  };

  return (
    <PageContainer
      header={{
        title: '跨集群配置对比',
        subTitle: '拉取两个集群中同名资源的 YAML，并高亮差异',
      }}
    >
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Card>
          <Form form={form} layout="vertical">
            <Row gutter={16} align="bottom">
              <Col xs={24} md={10}>
                <Form.Item
                  name="leftCluster"
                  label="左侧集群"
                  rules={[{ required: true, message: '请选择左侧集群' }]}
                >
                  <Select
                    showSearch
                    options={clusterOptions}
                    placeholder="选择集群"
                    optionFilterProp="label"
                  />
                </Form.Item>
                {!isClusterScoped && (
                  <Form.Item
                    name="leftNamespace"
                    label="左侧命名空间"
                    rules={[{ required: true, message: '请选择命名空间' }]}
                  >
                    <Select
                      showSearch
                      options={leftNsOptions}
                      placeholder="命名空间"
                      optionFilterProp="label"
                    />
                  </Form.Item>
                )}
              </Col>
              <Col xs={24} md={4} style={{ textAlign: 'center', paddingBottom: 24 }}>
                <Button icon={<SwapOutlined />} onClick={swapClusters}>
                  交换
                </Button>
              </Col>
              <Col xs={24} md={10}>
                <Form.Item
                  name="rightCluster"
                  label="右侧集群"
                  rules={[{ required: true, message: '请选择右侧集群' }]}
                >
                  <Select
                    showSearch
                    options={clusterOptions}
                    placeholder="选择集群"
                    optionFilterProp="label"
                  />
                </Form.Item>
                {!isClusterScoped && (
                  <Form.Item
                    name="rightNamespace"
                    label="右侧命名空间"
                    rules={[{ required: true, message: '请选择命名空间' }]}
                  >
                    <Select
                      showSearch
                      options={rightNsOptions}
                      placeholder="命名空间"
                      optionFilterProp="label"
                    />
                  </Form.Item>
                )}
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} md={8}>
                <Form.Item
                  name="kind"
                  label="资源类型"
                  rules={[{ required: true, message: '请选择类型' }]}
                >
                  <Select
                    showSearch
                    options={ALL_KIND_OPTIONS}
                    optionFilterProp="label"
                  />
                </Form.Item>
              </Col>
              <Col xs={24} md={10}>
                <Form.Item
                  name="name"
                  label="资源名称"
                  rules={[{ required: true, message: '请输入资源名称' }]}
                >
                  <Input placeholder="如 nginx" allowClear />
                </Form.Item>
              </Col>
              <Col xs={24} md={6}>
                <Form.Item label=" ">
                  <Button
                    type="primary"
                    icon={<DiffOutlined />}
                    loading={loading}
                    onClick={handleCompare}
                    block
                  >
                    加载并对比
                  </Button>
                </Form.Item>
              </Col>
            </Row>
          </Form>
        </Card>

        {compared && (
          <>
            {(leftError || rightError) && (
              <Alert
                type="warning"
                showIcon
                message={
                  <Space direction="vertical" size={0}>
                    {leftError && <Text>左侧：{leftError}</Text>}
                    {rightError && <Text>右侧：{rightError}</Text>}
                  </Space>
                }
              />
            )}
            <Alert
              type={summary.includes('一致') ? 'success' : 'info'}
              showIcon
              message={summary}
              action={
                <Button size="small" icon={<ReloadOutlined />} onClick={handleCompare}>
                  重新加载
                </Button>
              }
            />
            <Card
              title={
                <Space>
                  <span>YAML Diff</span>
                  <Text type="secondary">左侧 {leftCluster || '-'}</Text>
                  <Text type="secondary">右侧 {rightCluster || '-'}</Text>
                </Space>
              }
              bodyStyle={{ padding: 0 }}
            >
              {leftYaml || rightYaml ? (
                <YamlDiffEditor
                  original={leftYaml}
                  modified={rightYaml}
                  height="calc(100vh - 360px)"
                />
              ) : (
                <Empty description="没有可对比的 YAML" />
              )}
            </Card>
          </>
        )}
      </Space>
    </PageContainer>
  );
};

export default ClusterCompare;
