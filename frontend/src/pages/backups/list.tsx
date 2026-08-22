import React, { useCallback, useMemo, useRef, useState } from 'react';
import {
  Button,
  Card,
  Col,
  Descriptions,
  Form,
  Input,
  Modal,
  Popconfirm,
  Row,
  Space,
  Tag,
  message,
} from 'antd';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import type { ProColumns, ActionType } from '@ant-design/pro-components';
import {
  DatabaseOutlined,
  DeleteOutlined,
  DownloadOutlined,
  HistoryOutlined,
  PlusOutlined,
  ReloadOutlined,
  RollbackOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import dayjs from 'dayjs';
import {
  Backup,
  BackupStatus,
  StorageType,
  useDeleteBackupMutation,
  useListBackupsQuery,
  useRestoreBackupMutation,
} from '@/app/services/backup';
import { download } from '@/app/services/request';
import { BackupProgressPanel } from '@/components/ws';

const storageColorMap: Record<StorageType, string> = {
  Local: 'geekblue',
  S3: 'orange',
  NFS: 'purple',
};

const statusColorMap: Record<BackupStatus, string> = {
  Running: 'processing',
  Success: 'success',
  Failed: 'error',
  Pending: 'warning',
};

const statusLabelMap: Record<BackupStatus, string> = {
  Running: '运行中',
  Success: '成功',
  Failed: '失败',
  Pending: '等待中',
};

interface ListFilters {
  keyword?: string;
  code?: string;
  namespace?: string;
  kind?: string;
  storageType?: StorageType;
  status?: BackupStatus;
  page?: number;
  size?: number;
}

const BackupList: React.FC = () => {
  const navigate = useNavigate();
  const actionRef = useRef<ActionType>();
  const [searchParams] = useSearchParams();
  const codeParam = searchParams.get('code') || undefined;

  const [filters, setFilters] = useState<ListFilters>({
    keyword: '',
    code: codeParam,
    namespace: '',
    kind: '',
    storageType: undefined,
    status: undefined,
    page: 1,
    size: 10,
  });

  const { data, refetch, isFetching } = useListBackupsQuery(filters, {
    refetchOnMountOrArgChange: true,
  });

  const [deleteBackup] = useDeleteBackupMutation();
  const [restoreBackup, { isLoading: restoreLoading }] = useRestoreBackupMutation();

  const [restoreOpen, setRestoreOpen] = useState(false);
  const [restoringRecord, setRestoringRecord] = useState<Backup | null>(null);
  const [restoreForm] = Form.useForm();

  const handleReload = useCallback(() => refetch(), [refetch]);

  const handleOpenRestore = (record: Backup) => {
    setRestoringRecord(record);
    restoreForm.setFieldsValue({
      targetCluster: record.code,
      targetNamespace: record.namespace,
    });
    setRestoreOpen(true);
  };

  const handleConfirmRestore = async () => {
    if (!restoringRecord) return;
    try {
      const values = await restoreForm.validateFields();
      const res = await restoreBackup({
        id: restoringRecord.id,
        body: values,
      }).unwrap();
      message.success(res.message || '恢复任务已提交');
      setRestoreOpen(false);
      handleReload();
    } catch {
      // interceptor
    }
  };

  const handleDownload = async (record: Backup) => {
    try {
      const filename = `backup-${record.code}-${record.kind}-${record.name || record.id}-${
        record.id
      }.yaml`;
      const url = record.downloadUrl || `/backups/${record.id}/download`;
      await download(url, filename);
      message.success('开始下载');
    } catch {
      message.error('下载失败');
    }
  };

  const columns: ProColumns<Backup>[] = useMemo(
    () => [
      {
        title: '备份时间',
        dataIndex: 'createdAt',
        key: 'createdAt',
        width: 180,
        sorter: true,
        render: (v: string | undefined, record) => (
          <Space direction="vertical" size={0}>
            <span>{v ? dayjs(v).format('YYYY-MM-DD HH:mm:ss') : '-'}</span>
            {record.finishedAt && (
              <span style={{ color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>
                完成: {dayjs(record.finishedAt).format('MM-DD HH:mm:ss')}
              </span>
            )}
          </Space>
        ),
      },
      {
        title: '操作人',
        dataIndex: 'operator',
        key: 'operator',
        width: 120,
        render: (v) => v || '-',
      },
      {
        title: '集群',
        dataIndex: 'code',
        key: 'code',
        width: 140,
        render: (v, record) => (
          <Space>
            <Tag color="blue">{v}</Tag>
            {record.clusterName && (
              <span style={{ color: 'rgba(0,0,0,0.65)' }}>{record.clusterName}</span>
            )}
          </Space>
        ),
      },
      {
        title: '命名空间',
        dataIndex: 'namespace',
        key: 'namespace',
        width: 130,
        render: (v) => v || <Tag>（集群级）</Tag>,
      },
      {
        title: '对象类型 / 名称',
        key: 'object',
        width: 240,
        render: (_v, record) => (
          <Space direction="vertical" size={0}>
            <Tag color="purple">{record.kind}</Tag>
            <a
              style={{ wordBreak: 'break-all' }}
              onClick={() => {
                const ns = record.namespace ? encodeURIComponent(record.namespace) : '_';
                navigate(
                  `/resources/${record.code}/${encodeURIComponent(record.apiVersion)}/${encodeURIComponent(
                    record.kind,
                  )}/${ns}/${encodeURIComponent(record.name)}/edit`,
                );
              }}
            >
              {record.name}
            </a>
          </Space>
        ),
      },
      {
        title: '存储类型',
        dataIndex: 'storageType',
        key: 'storageType',
        width: 110,
        render: (v: StorageType) => (
          <Tag color={storageColorMap[v] || 'default'}>{v}</Tag>
        ),
      },
      {
        title: '大小',
        dataIndex: 'size',
        key: 'size',
        width: 110,
        render: (v: number | undefined) => {
          if (!v) return '-';
          if (v < 1024) return `${v} B`;
          if (v < 1024 * 1024) return `${(v / 1024).toFixed(1)} KB`;
          if (v < 1024 * 1024 * 1024) return `${(v / 1024 / 1024).toFixed(2)} MB`;
          return `${(v / 1024 / 1024 / 1024).toFixed(2)} GB`;
        },
      },
      {
        title: '状态',
        dataIndex: 'status',
        key: 'status',
        width: 120,
        render: (v: BackupStatus) => (
          <Tag color={statusColorMap[v]}>{statusLabelMap[v] || v}</Tag>
        ),
      },
      {
        title: '操作',
        key: 'actions',
        width: 280,
        fixed: 'right',
        render: (_v, record) => (
          <Space size="small" wrap>
            <Button
              type="link"
              size="small"
              icon={<RollbackOutlined />}
              onClick={() => handleOpenRestore(record)}
              disabled={record.status !== 'Success'}
            >
              恢复
            </Button>
            <Button
              type="link"
              size="small"
              icon={<DownloadOutlined />}
              onClick={() => handleDownload(record)}
              disabled={record.status !== 'Success'}
            >
              下载
            </Button>
            <Button
              type="link"
              size="small"
              icon={<HistoryOutlined />}
              onClick={() => {
                const ns = record.namespace ? encodeURIComponent(record.namespace) : '_';
                navigate(
                  `/resources/${record.code}/${encodeURIComponent(record.apiVersion)}/${encodeURIComponent(
                    record.kind,
                  )}/${ns}/${encodeURIComponent(record.name)}/edit?tab=versions`,
                );
              }}
            >
              资源版本
            </Button>
            <Popconfirm
              title={`确认删除备份「${record.name || record.id}」？`}
              description="删除后备份文件也将被清理"
              okButtonProps={{ danger: true }}
              onConfirm={async () => {
                try {
                  await deleteBackup(record.id).unwrap();
                  message.success('删除成功');
                  handleReload();
                } catch {
                  // interceptor
                }
              }}
            >
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                删除
              </Button>
            </Popconfirm>
          </Space>
        ),
      },
    ],
    [deleteBackup, handleReload, navigate],
  );

  return (
    <PageContainer>
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} lg={10}>
          <Card
            size="small"
            title={
              <Space>
                <ThunderboltOutlined />
                <span>备份/恢复实时进度</span>
              </Space>
            }
            bodyStyle={{ padding: 0 }}
          >
            <BackupProgressPanel
              compact
              clusterCode={codeParam}
              clearOnMount={false}
              height={140}
            />
          </Card>
        </Col>
      </Row>

      <ProTable<Backup>
        headerTitle={
          <Space>
            <DatabaseOutlined />
            <span>备份列表</span>
          </Space>
        }
        actionRef={actionRef}
        rowKey="id"
        columns={columns}
        loading={isFetching}
        search={{
          labelWidth: 'auto',
          defaultCollapsed: false,
          searchText: '搜索',
          resetText: '重置',
        }}
        form={{
          initialValues: {
            keyword: filters.keyword,
            code: filters.code,
            namespace: filters.namespace,
            kind: filters.kind,
            storageType: filters.storageType,
            status: filters.status,
          },
        }}
        onSubmit={(values) => {
          setFilters((prev) => ({
            ...prev,
            keyword: (values.keyword as string) || '',
            code: (values.code as string) || undefined,
            namespace: (values.namespace as string) || '',
            kind: (values.kind as string) || '',
            storageType: (values.storageType as StorageType) || undefined,
            status: (values.status as BackupStatus) || undefined,
            page: 1,
          }));
        }}
        onReset={() => {
          setFilters((prev) => ({
            ...prev,
            keyword: '',
            code: undefined,
            namespace: '',
            kind: '',
            storageType: undefined,
            status: undefined,
            page: 1,
          }));
        }}
        toolBarRender={() => [
          <Button key="refresh" icon={<ReloadOutlined />} onClick={handleReload}>
            刷新
          </Button>,
          <Button
            key="create"
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => navigate('/backups/create')}
          >
            新建备份
          </Button>,
        ]}
        dataSource={data?.items || []}
        pagination={{
          current: filters.page || 1,
          pageSize: filters.size || 10,
          total: data?.total || 0,
          showSizeChanger: true,
          onChange: (page, size) => setFilters((prev) => ({ ...prev, page, size })),
        }}
        scroll={{ x: 1500 }}
        options={{
          reload: handleReload,
          density: true,
          fullScreen: true,
          setting: true,
        }}
      />

      <Modal
        title={
          <Space>
            <RollbackOutlined />
            <span>恢复备份</span>
            {restoringRecord && <Tag color="blue">#{restoringRecord.id.slice(0, 8)}</Tag>}
          </Space>
        }
        open={restoreOpen}
        onCancel={() => setRestoreOpen(false)}
        destroyOnClose
        okText="确认恢复"
        confirmLoading={restoreLoading}
        onOk={handleConfirmRestore}
        width={560}
      >
        {restoringRecord && (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <Descriptions size="small" column={2} bordered>
              <Descriptions.Item label="集群">{restoringRecord.code}</Descriptions.Item>
              <Descriptions.Item label="命名空间">
                {restoringRecord.namespace || '（集群级）'}
              </Descriptions.Item>
              <Descriptions.Item label="Kind">{restoringRecord.kind}</Descriptions.Item>
              <Descriptions.Item label="对象名称">{restoringRecord.name}</Descriptions.Item>
              <Descriptions.Item label="存储类型" span={2}>
                {restoringRecord.storageType}
              </Descriptions.Item>
            </Descriptions>
            <Form form={restoreForm} layout="vertical" preserve={false}>
              <Form.Item
                label="目标集群"
                name="targetCluster"
                rules={[{ required: true, message: '请输入目标集群编码' }]}
                help="可以恢复到当前集群或其它已导入的集群"
              >
                <Input placeholder="请输入目标集群编码" allowClear />
              </Form.Item>
              <Form.Item
                label="目标命名空间"
                name="targetNamespace"
                help="留空则使用备份时的命名空间；集群级资源无需填写"
              >
                <Input placeholder="命名空间（可留空）" allowClear />
              </Form.Item>
            </Form>
          </Space>
        )}
      </Modal>
    </PageContainer>
  );
};

export default BackupList;
