import React, { useMemo } from 'react';
import {
  Button,
  Card,
  Form,
  Radio,
  Space,
  message,
} from 'antd';
import {
  PageContainer,
  ProForm,
  ProFormItem,
  ProFormSelect,
  ProFormText,
} from '@ant-design/pro-components';
import {
  ArrowLeftOutlined,
  CloudServerOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import {
  useListClustersQuery,
} from '@/app/services/cluster';
import { useListNamespacesQuery } from '@/app/services/resource';
import { useCreateBackupMutation } from '@/app/services/backup';
import { useAppSelector } from '@/app/store';

interface CreateFormValues {
  code: string;
  namespace?: string;
  scope: 'object' | 'namespace';
  apiVersion: string;
  kind: string;
  name?: string;
  storageType: 'Local' | 'S3' | 'NFS';
}

const COMMON_KINDS = [
  { apiVersion: 'apps/v1', kind: 'Deployment' },
  { apiVersion: 'apps/v1', kind: 'StatefulSet' },
  { apiVersion: 'apps/v1', kind: 'DaemonSet' },
  { apiVersion: 'batch/v1', kind: 'Job' },
  { apiVersion: 'batch/v1', kind: 'CronJob' },
  { apiVersion: 'v1', kind: 'ConfigMap' },
  { apiVersion: 'v1', kind: 'Secret' },
  { apiVersion: 'v1', kind: 'Service' },
  { apiVersion: 'v1', kind: 'PersistentVolume' },
  { apiVersion: 'v1', kind: 'PersistentVolumeClaim' },
  { apiVersion: 'storage.k8s.io/v1', kind: 'StorageClass' },
  { apiVersion: 'networking.k8s.io/v1', kind: 'Ingress' },
];

const BackupCreate: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const selectedClusterCode = useAppSelector((s) => s.app.selectedClusterCode);
  const [form] = Form.useForm<CreateFormValues>();

  const preCode = searchParams.get('code') || selectedClusterCode || '';
  const preNamespace = searchParams.get('namespace') || '';
  const preKind = searchParams.get('kind') || '';
  const preApiVersion = searchParams.get('apiVersion') || '';
  const preName = searchParams.get('name') || '';

  const { data: clusterData } = useListClustersQuery(undefined, {
    refetchOnMountOrArgChange: true,
  });
  const clusters = clusterData?.list || [];

  const codeValue = Form.useWatch('code', form) as string | undefined;
  const scopeValue = Form.useWatch('scope', form) as 'object' | 'namespace' | undefined;
  const kindValue = Form.useWatch('kind', form) as string | undefined;

  const kindApiVersion = useMemo(() => {
    if (preApiVersion && preKind) {
      return preApiVersion;
    }
    const found = COMMON_KINDS.find((k) => k.kind === kindValue);
    return found?.apiVersion || '';
  }, [kindValue, preApiVersion, preKind]);

  const { data: namespaceData } = useListNamespacesQuery(codeValue || '', {
    skip: !codeValue,
    refetchOnMountOrArgChange: true,
  });

  const [createBackup, { isLoading }] = useCreateBackupMutation();

  const initialValues: CreateFormValues = {
    code: preCode,
    namespace: preNamespace || undefined,
    scope: 'object',
    apiVersion: kindApiVersion,
    kind: preKind,
    name: preName,
    storageType: 'Local',
  };

  const handleSubmit = async (values: CreateFormValues) => {
    try {
      if (!values.code) {
        message.error('请选择集群');
        return;
      }
      const av = values.apiVersion || kindApiVersion;
      if (!av) {
        message.error('无法确定 apiVersion，请明确选择资源类型');
        return;
      }
      const body = {
        namespace: values.scope === 'namespace' ? values.namespace || undefined : values.namespace,
        apiVersion: av,
        kind: values.kind,
        name: values.scope === 'namespace' ? '' : values.name || '',
        storageType: values.storageType,
        scope: values.scope,
      };
      if (values.scope === 'object' && !body.name) {
        message.error('单对象模式需要填写对象名称');
        return;
      }
      if (values.scope === 'namespace' && !body.namespace) {
        message.error('命名空间级模式需要指定命名空间');
        return;
      }
      await createBackup({ code: values.code, body }).unwrap();
      message.success('备份任务已提交，刷新列表查看状态');
      navigate(`/backups/list?code=${encodeURIComponent(values.code)}`);
    } catch {
      // handled by interceptor
    }
  };

  return (
    <PageContainer
      onBack={() => navigate(-1)}
      backIcon={<ArrowLeftOutlined />}
      title={
        <Space>
          <CloudServerOutlined />
          <span>新建备份</span>
        </Space>
      }
    >
      <Card bordered={false}>
        <ProForm<CreateFormValues>
          form={form}
          layout="vertical"
          initialValues={initialValues}
          onFinish={handleSubmit}
          submitter={{
            render: (_props, doms) => (
              <Space>
                <Button onClick={() => navigate('/backups/list')}>返回</Button>
                {doms[1]}
              </Space>
            ),
            searchConfig: {
              submitText: '提交备份任务',
              resetText: '重置',
            },
            submitButtonProps: {
              loading: isLoading,
              icon: <PlusOutlined />,
              type: 'primary',
            },
          }}
        >
          <ProFormSelect
            name="code"
            label="集群"
            placeholder="请选择要备份的集群"
            rules={[{ required: true, message: '请选择集群' }]}
            options={clusters.map((c) => ({
              value: c.code,
              label: `${c.name} (${c.code})`,
            }))}
            fieldProps={{
              showSearch: true,
              optionFilterProp: 'label',
            }}
          />

          <ProFormItem
            name="scope"
            label="备份范围"
            rules={[{ required: true, message: '请选择备份范围' }]}
          >
            <Radio.Group optionType="button" buttonStyle="solid">
              <Radio.Button value="object">单对象</Radio.Button>
              <Radio.Button value="namespace">命名空间级</Radio.Button>
            </Radio.Group>
          </ProFormItem>

          <ProFormSelect
            name="namespace"
            label="命名空间"
            placeholder="集群级资源可不填；命名空间级必填"
            disabled={!codeValue}
            options={(namespaceData || []).map((n) => ({ value: n, label: n }))}
            rules={
              scopeValue === 'namespace'
                ? [{ required: true, message: '命名空间级模式请选择命名空间' }]
                : []
            }
            fieldProps={{
              allowClear: true,
              showSearch: true,
            }}
          />

          <ProFormSelect
            name="kind"
            label="对象类别 (Kind)"
            placeholder="请选择对象类型"
            rules={[{ required: true, message: '请选择对象类别' }]}
            options={COMMON_KINDS.map((k) => ({
              value: k.kind,
              label: `${k.kind} (${k.apiVersion})`,
            }))}
            fieldProps={{
              showSearch: true,
              optionFilterProp: 'label',
            }}
          />

          {scopeValue === 'object' && (
            <ProFormText
              name="name"
              label="对象名称"
              placeholder="请输入要备份的对象名称"
              rules={[{ required: true, message: '单对象模式下必填对象名称' }]}
              fieldProps={{ maxLength: 255 }}
            />
          )}

          <ProFormSelect
            name="storageType"
            label="存储类型"
            initialValue="Local"
            rules={[{ required: true, message: '请选择存储类型' }]}
            options={[
              { value: 'Local', label: 'Local - 本地存储' },
              { value: 'S3', label: 'S3 - 对象存储' },
              { value: 'NFS', label: 'NFS - 网络文件系统' },
            ]}
          />

          <ProFormText
            name="apiVersion"
            label="API Version"
            placeholder="自动从类别推导，可手动覆盖"
            tooltip="如 apps/v1, batch/v1, v1, storage.k8s.io/v1 等"
            fieldProps={{
              placeholder: kindApiVersion || '如 apps/v1 / v1 / batch/v1',
            }}
          />
        </ProForm>
      </Card>
    </PageContainer>
  );
};

export default BackupCreate;
