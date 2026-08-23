import React, { useMemo } from 'react';
import {
  Button,
  Card,
  Form,
  Radio,
  Space,
  message,
  Checkbox,
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
import { CreateBackupBody, useCreateBackupMutation } from '@/app/services/backup';
import type { Cluster } from '@/app/services/cluster';
import { useAppSelector } from '@/app/store';

type BackupMode = 'object' | 'namespace' | 'namespace_batch';

interface CreateFormValues {
  code: string;
  namespace?: string;
  namespaces?: string[];
  scope: BackupMode;
  apiVersion: string;
  kind: string;
  kindFilter?: string[];
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
  const clusters = clusterData?.items || [];

  const codeValue = Form.useWatch('code', form) as string | undefined;
  const scopeValue = Form.useWatch('scope', form) as BackupMode | undefined;
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
    namespaces: preNamespace ? [preNamespace] : undefined,
    scope: 'object',
    apiVersion: kindApiVersion,
    kind: preKind,
    kindFilter: preKind ? [preKind] : undefined,
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

      // 根据 scope 决定 mode：
      //   object => single
      //   namespace => namespace_batch（单命名空间单 kind，走 namespaces=[ns], kind_filter=[kind]）
      //   namespace_batch => namespace_batch
      const isBatch =
        values.scope === 'namespace' || values.scope === 'namespace_batch';

      const body: CreateBackupBody = {
        storageType: values.storageType,
        scope: values.scope,
      };

      if (isBatch) {
        body.mode = 'namespace_batch';
        // 组装 namespaces：
        let nsList: string[] = [];
        if (values.scope === 'namespace_batch') {
          nsList = values.namespaces || [];
        } else {
          // namespace 单命名空间模式：兼容旧行为，取 namespace 单值
          nsList = values.namespace ? [values.namespace] : [];
        }
        if (nsList.length === 0) {
          message.error(
            values.scope === 'namespace_batch'
              ? '请至少选择一个命名空间'
              : '请选择命名空间',
          );
          return;
        }
        body.namespaces = nsList;

        // 组装 kind_filter：
        let kfList: string[] = [];
        if (values.scope === 'namespace_batch') {
          kfList = values.kindFilter || [];
        } else {
          kfList = values.kind ? [values.kind] : [];
        }
        if (kfList.length === 0) {
          message.error(
            values.scope === 'namespace_batch'
              ? '请至少选择一种资源类型'
              : '请选择对象类别',
          );
          return;
        }
        body.kind_filter = kfList;
      } else {
        // 单对象模式：
        body.mode = 'single';
        if (!av) {
          message.error('无法确定 apiVersion，请明确选择资源类型');
          return;
        }
        body.namespace = values.namespace || '';
        body.target_kind = values.kind;
        body.target_name = values.name || '';
        body.apiVersion = av;
        if (!body.target_kind || !body.target_name) {
          message.error('单对象模式需要填写对象类别和名称');
          return;
        }
      }

      await createBackup({ code: values.code, body }).unwrap();
      message.success('备份任务已提交，刷新列表查看状态');
      navigate(`/backups/list?code=${encodeURIComponent(values.code)}`);
    } catch {
      // handled by interceptor
    }
  };

  const namespaceOptions = (namespaceData || []).map((n) => ({
    value: n,
    label: n,
  }));

  const kindOptions = COMMON_KINDS.map((k) => ({
    value: k.kind,
    label: `${k.kind} (${k.apiVersion})`,
  }));

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
            options={clusters.map((c: Cluster) => ({
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
              <Radio.Button value="namespace_batch">命名空间批量</Radio.Button>
            </Radio.Group>
          </ProFormItem>

          {/* 命名空间批量：多选命名空间 */}
          {scopeValue === 'namespace_batch' && (
            <Form.Item
              name="namespaces"
              label="命名空间（多选）"
              rules={[
                { required: true, message: '请至少选择一个命名空间' },
              ]}
            >
              <Checkbox.Group
                style={{ width: '100%' }}
                options={namespaceOptions}
                disabled={!codeValue}
              />
            </Form.Item>
          )}

          {/* 命名空间级 / 单对象：单选命名空间 */}
          {(scopeValue === 'object' || scopeValue === 'namespace') && (
            <ProFormSelect
              name="namespace"
              label="命名空间"
              placeholder={
                scopeValue === 'namespace'
                  ? '请选择命名空间'
                  : '集群级资源可不填；命名空间级必填'
              }
              disabled={!codeValue}
              options={namespaceOptions}
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
          )}

          {/* 命名空间批量：多选资源类型 */}
          {scopeValue === 'namespace_batch' && (
            <Form.Item
              name="kindFilter"
              label="资源类型（Kind 多选）"
              rules={[
                { required: true, message: '请至少选择一种资源类型' },
              ]}
            >
              <Checkbox.Group
                style={{ width: '100%' }}
                options={kindOptions}
              />
            </Form.Item>
          )}

          {/* 单对象 / 命名空间级：单 Kind 选择 */}
          {(scopeValue === 'object' || scopeValue === 'namespace') && (
            <ProFormSelect
              name="kind"
              label="对象类别 (Kind)"
              placeholder="请选择对象类型"
              rules={[{ required: true, message: '请选择对象类别' }]}
              options={kindOptions}
              fieldProps={{
                showSearch: true,
                optionFilterProp: 'label',
              }}
            />
          )}

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
              { value: 'NFS', label: 'NFS - 网络文件系统' },
              { value: 'S3', label: 'S3 - 对象存储（暂未开放）' },
            ]}
          />

          {scopeValue === 'object' && (
            <ProFormText
              name="apiVersion"
              label="API Version"
              placeholder="自动从类别推导，可手动覆盖"
              tooltip="如 apps/v1, batch/v1, v1, storage.k8s.io/v1 等"
              fieldProps={{
                placeholder: kindApiVersion || '如 apps/v1 / v1 / batch/v1',
              }}
            />
          )}

          {scopeValue === 'namespace_batch' && (
            <Card
              size="small"
              type="inner"
              title="批量备份说明"
              style={{ marginTop: 8 }}
            >
              <p style={{ margin: 0, color: 'rgba(0,0,0,0.65)' }}>
                命名空间批量模式会遍历已勾选的「命名空间 × 资源类型」组合，
                对匹配到的每个 K8s 对象分别导出 YAML 并合并归档。
                任务进度会按对象数量细粒度反馈。
              </p>
            </Card>
          )}
        </ProForm>
      </Card>
    </PageContainer>
  );
};

export default BackupCreate;
