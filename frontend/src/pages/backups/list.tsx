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
  Select,
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
  downloadBackupById,
  useDeleteBackupMutation,
  useListBackupsQuery,
  useRestoreBackupMutation,
} from '@/app/services/backup';
import { useListClustersQuery } from '@/app/services/cluster';
import { BackupProgressPanel } from '@/components/ws';
import { usePermission } from '@/hooks/usePermission';
import { useBackupProgress } from '@/hooks/useBackupProgress';
import { Progress } from 'antd';

const storageColorMap: Record<string, string> = {
  local: 'geekblue',
  s3: 'orange',
  nfs: 'purple',
};

const statusColorMap: Record<string, string> = {
  running: 'processing',
  success: 'success',
  failed: 'error',
  pending: 'warning',
  cancelled: 'default',
};

const statusLabelMap: Record<string, string> = {
  running: '运行中',
  success: '成功',
  failed: '失败',
  pending: '等待中',
  cancelled: '已取消',
};

const modeLabelMap: Record<string, string> = {
  single: '单对象',
  namespace_batch: '命名空间批量',
  restore: '恢复任务',
};

interface ListFilters {
  keyword?: string;
  cluster_code?: string;
  backup_type?: string;
  status?: BackupStatus;
  page?: number;
  size?: number;
}

const BackupList: React.FC = () => {
  const navigate = useNavigate();
  const actionRef = useRef<ActionType>();
  const [searchParams] = useSearchParams();
  const codeParam = searchParams.get('code') || undefined;
  const { hasPerm } = usePermission();
  // 权限码与后端 seed 及 routes_backup.go 对齐：
  // 查看与下载接口均要求 backup:list（无独立 backup:view/backup:download 权限点）
  const canView = hasPerm('backup:list');
  const canCreate = hasPerm('backup:create');
  const canRestore = hasPerm('backup:restore');
  const canDownload = hasPerm('backup:list');
  const canDelete = hasPerm('backup:delete');

  // 集群列表，用于恢复表单的目标集群下拉
  const { data: clustersData } = useListClustersQuery(
    { page: 1, size: 200 },
    { refetchOnMountOrArgChange: true },
  );
  const clusterOptions = useMemo(
    () =>
      (clustersData?.items || []).map((c) => ({
        label: c.name ? `${c.name} (${c.code})` : c.code,
        value: c.code,
      })),
    [clustersData],
  );

  const [filters, setFilters] = useState<ListFilters>({
    keyword: '',
    cluster_code: codeParam,
    backup_type: undefined,
    status: undefined,
    page: 1,
    size: 10,
  });

  const { data, refetch, isFetching } = useListBackupsQuery(filters, {
    skip: !canView,
    refetchOnMountOrArgChange: true,
  });

  // WebSocket 实时进度：按 task_id 分组取最新一条，用于表格行内进度覆盖
  const { byTaskId } = useBackupProgress({
    clusterCode: codeParam,
    clearOnMount: false,
    mode: 'latest-per-task',
  });

  const [deleteBackup] = useDeleteBackupMutation();
  const [restoreBackup, { isLoading: restoreLoading }] = useRestoreBackupMutation();

  const [restoreOpen, setRestoreOpen] = useState(false);
  const [restoringRecord, setRestoringRecord] = useState<Backup | null>(null);
  const [restoreForm] = Form.useForm();

  const handleReload = useCallback(() => refetch(), [refetch]);

  const handleOpenRestore = (record: Backup) => {
    if (!canRestore) return;
    setRestoringRecord(record);
    restoreForm.setFieldsValue({
      targetCluster: record.cluster_code,
      targetNamespace: record.namespace || undefined,
      mode: 'create-new',
    });
    setRestoreOpen(true);
  };

  const handleConfirmRestore = async () => {
    if (!restoringRecord || !canRestore) return;
    try {
      const values = await restoreForm.validateFields();
      const res = await restoreBackup({
        id: restoringRecord.id,
        body: {
          target_cluster_code: values.targetCluster,
          target_namespace: values.targetNamespace || undefined,
          mode: values.mode,
        },
      }).unwrap();
      const target = values.targetCluster || restoringRecord.cluster_code;
      message.success(`恢复任务 #${res.id} 已提交，切到集群 ${target} 查看进度`);
      setRestoreOpen(false);
      handleReload();
    } catch {
      // interceptor
    }
  };

  const handleDownload = async (record: Backup) => {
    if (!canDownload) {
      message.error('无备份下载权限');
      return;
    }
    if (record.backup_type === 'namespace_batch') {
      message.warning('批量备份暂不支持下载（尚未打包为单个文件）');
      return;
    }
    try {
      await downloadBackupById(record);
      message.success('开始下载');
    } catch {
      // request.download 已提示
    }
  };

  const columns: ProColumns<Backup>[] = useMemo(
    () => [
      {
        title: '备份时间',
        dataIndex: 'created_at',
        key: 'created_at',
        width: 180,
        sorter: true,
        render: (_dom, record) => (
          <Space direction="vertical" size={0}>
            <span>{record.created_at ? dayjs(record.created_at).format('YYYY-MM-DD HH:mm:ss') : '-'}</span>
            {record.completed_at && (
              <span style={{ color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>
                完成: {dayjs(record.completed_at).format('MM-DD HH:mm:ss')}
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
        render: (_dom, record) => record.operator || '-',
      },
      {
        title: '集群',
        dataIndex: 'cluster_code',
        key: 'cluster_code',
        width: 140,
        render: (_dom, record) => <Tag color="blue">{record.cluster_code}</Tag>,
      },
      {
        title: '类型',
        key: 'taskType',
        width: 100,
        render: (_dom, record) => {
          const isRestore = record.backup_type === 'restore';
          return (
            <Tag color={isRestore ? 'orange' : 'blue'}>
              {isRestore ? '恢复' : '备份'}
            </Tag>
          );
        },
      },
      {
        title: '备份模式',
        dataIndex: 'backup_type',
        key: 'backup_type',
        width: 120,
        render: (_dom, record) => {
          const displayMode = record.backup_type || 'single';
          return <Tag color={displayMode === 'namespace_batch' ? 'magenta' : 'cyan'}>
            {modeLabelMap[displayMode] || displayMode || '单对象'}
          </Tag>;
        },
      },
      {
        title: '命名空间',
        dataIndex: 'namespace',
        key: 'namespace',
        width: 180,
        render: (_dom, record) => {
          // 批量多命名空间时后端存 "multi:<count>" 文本
          const ns = record.namespace;
          if (ns && ns.startsWith('multi:')) {
            return <Tag color="geekblue">{ns.replace('multi:', '多命名空间 ×')}</Tag>;
          }
          return ns || <Tag>（集群级）</Tag>;
        },
      },
      {
        title: '对象类型 / 名称',
        key: 'object',
        width: 260,
        render: (_v, record) => {
          // 批量模式：target_kind 仅存第一个 kind（详情在后端 Redis payload，不随列表返回）
          return (
            <Space direction="vertical" size={0}>
              {record.target_kind && <Tag color="purple">{record.target_kind}</Tag>}
              {record.target_name && <span style={{ wordBreak: 'break-all' }}>{record.target_name}</span>}
            </Space>
          );
        },
      },
      {
        title: '存储类型',
        dataIndex: 'storage_type',
        key: 'storage_type',
        width: 110,
        render: (_dom, record) => (
          <Tag color={storageColorMap[record.storage_type] || 'default'}>{record.storage_type}</Tag>
        ),
      },
      {
        title: '大小',
        dataIndex: 'size_bytes',
        key: 'size_bytes',
        width: 110,
        render: (_dom, record) => {
          const v = record.size_bytes;
          if (!v) return '-';
          if (v < 1024) return `${v} B`;
          if (v < 1024 * 1024) return `${(v / 1024).toFixed(1)} KB`;
          if (v < 1024 * 1024 * 1024) return `${(v / 1024 / 1024).toFixed(2)} MB`;
          return `${(v / 1024 / 1024 / 1024).toFixed(2)} GB`;
        },
      },
      {
        title: '状态/进度',
        dataIndex: 'status',
        key: 'status',
        width: 200,
        render: (_dom, record) => {
          const live = byTaskId[String(record.id)];
          const liveStage = live?.stage;
          const liveProgress = live?.progress ?? 0;
          const liveMessage = live?.message;
          const liveError = live?.error;

          // 若 WS 有实时推送且非终态 → 用实时数据覆盖，附带进度条
          const useLive =
            !!live &&
            liveStage !== 'completed' &&
            liveStage !== 'failed';

          if (useLive && liveStage) {
            const stageColor: Record<string, string> = {
              pending: 'warning',
              exporting: 'processing',
              uploading: 'purple',
              restoring: 'orange',
            };
            const stageLabel: Record<string, string> = {
              pending: '等待中',
              exporting: '导出中',
              uploading: '上传中',
              restoring: '恢复中',
            };
            return (
              <Space direction="vertical" size={2} style={{ width: '100%' }}>
                <Tag color={stageColor[liveStage] || 'processing'}>
                  {stageLabel[liveStage] || liveStage}
                </Tag>
                <Progress
                  percent={Math.min(100, Math.max(0, liveProgress))}
                  size="small"
                  status="active"
                  showInfo
                />
                {liveMessage && (
                  <div style={{ fontSize: 12, color: 'rgba(0,0,0,0.65)' }}>{liveMessage}</div>
                )}
                {liveError && <div style={{ fontSize: 12, color: '#ff4d4f' }}>{liveError}</div>}
              </Space>
            );
          }

          // 终态或无推送 → 回退到静态 status + 可能的最新 message（WS completed/failed 可能带详情）
          const finalStage = liveStage === 'completed' || liveStage === 'failed' ? liveStage : undefined;
          const finalLabel =
            finalStage === 'completed'
              ? '已完成'
              : finalStage === 'failed'
                ? '失败'
                : statusLabelMap[record.status] || record.status;
          const finalColor: string =
            finalStage === 'completed'
              ? 'success'
              : finalStage === 'failed'
                ? 'error'
                : statusColorMap[record.status];
          return (
            <Space direction="vertical" size={2} style={{ width: '100%' }}>
              <Tag color={finalColor}>{finalLabel}</Tag>
              {live?.message && finalStage && (
                <div style={{ fontSize: 12, color: 'rgba(0,0,0,0.65)' }}>{live.message}</div>
              )}
              {live?.error && (
                <div style={{ fontSize: 12, color: '#ff4d4f' }}>{live.error}</div>
              )}
            </Space>
          );
        },
      },
      {
        title: '操作',
        key: 'actions',
        width: 280,
        fixed: 'right',
        render: (_v, record) => {
          const isBatch = record.backup_type === 'namespace_batch' || record.backup_type === 'restore';
          return (
            <Space size="small" wrap>
              <Button
                type="link"
                size="small"
                icon={<RollbackOutlined />}
                onClick={() => handleOpenRestore(record)}
                disabled={record.status !== 'success' || isBatch || !canRestore}
              >
                恢复
              </Button>
              <Button
                type="link"
                size="small"
                icon={<DownloadOutlined />}
                onClick={() => handleDownload(record)}
                disabled={record.status !== 'success' || isBatch || !canDownload}
              >
                下载
              </Button>
              <Button
                type="link"
                size="small"
                icon={<HistoryOutlined />}
                disabled={isBatch || !record.target_name}
                onClick={() => {
                  // BackupTask 不存 api_version，无法直达编辑页版本 Tab，退化为跳转资源列表并按 kind 过滤
                  navigate(
                    `/resources/list?cluster_code=${encodeURIComponent(record.cluster_code)}${
                      record.target_kind ? `&kind=${encodeURIComponent(record.target_kind)}` : ''
                    }${record.namespace ? `&namespace=${encodeURIComponent(record.namespace)}` : ''}`,
                  );
                }}
              >
                资源版本
              </Button>
              <Popconfirm
                title={`确认删除备份「${record.target_name || `#${record.id}`}」？`}
                description="删除后备份文件也将被清理"
                okButtonProps={{ danger: true, disabled: !canDelete }}
                onConfirm={async () => {
                  if (!canDelete) return;
                  try {
                    await deleteBackup(record.id).unwrap();
                    message.success('删除成功');
                    handleReload();
                  } catch {
                    // interceptor
                  }
                }}
              >
                <Button
                  type="link"
                  size="small"
                  danger
                  icon={<DeleteOutlined />}
                  disabled={!canDelete}
                >
                  删除
                </Button>
              </Popconfirm>
            </Space>
          );
        },
      },
    ],
    [byTaskId, deleteBackup, handleReload, navigate, canRestore, canDownload, canDelete],
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
            cluster_code: filters.cluster_code,
            backup_type: filters.backup_type,
            status: filters.status,
          },
        }}
        onSubmit={(values) => {
          setFilters((prev) => ({
            ...prev,
            keyword: (values.keyword as string) || '',
            cluster_code: (values.cluster_code as string) || undefined,
            backup_type: (values.backup_type as string) || undefined,
            status: (values.status as BackupStatus) || undefined,
            page: 1,
          }));
        }}
        onReset={() => {
          setFilters((prev) => ({
            ...prev,
            keyword: '',
            cluster_code: undefined,
            backup_type: undefined,
            status: undefined,
            page: 1,
          }));
        }}
        toolBarRender={() => [
          <Button key="refresh" icon={<ReloadOutlined />} onClick={handleReload}>
            刷新
          </Button>,
          canCreate && (
            <Button
              key="create"
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => navigate('/backups/create')}
            >
              新建备份
            </Button>
          ),
        ].filter(Boolean)}
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
            {restoringRecord && <Tag color="blue">#{restoringRecord.id}</Tag>}
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
              <Descriptions.Item label="集群">{restoringRecord.cluster_code}</Descriptions.Item>
              <Descriptions.Item label="命名空间">
                {restoringRecord.namespace || '（集群级）'}
              </Descriptions.Item>
              <Descriptions.Item label="Kind">{restoringRecord.target_kind || '-'}</Descriptions.Item>
              <Descriptions.Item label="对象名称">{restoringRecord.target_name || '-'}</Descriptions.Item>
              <Descriptions.Item label="存储类型" span={2}>
                {restoringRecord.storage_type}
              </Descriptions.Item>
            </Descriptions>
            <Form form={restoreForm} layout="vertical" preserve={false}>
              <Form.Item
                label="目标集群"
                name="targetCluster"
                rules={[{ required: true, message: '请选择目标集群' }]}
                help="可以恢复到当前集群或其它已导入的集群"
              >
                <Select
                  placeholder="请选择目标集群"
                  options={clusterOptions}
                  showSearch
                  optionFilterProp="label"
                  allowClear
                />
              </Form.Item>
              <Form.Item
                label="目标命名空间"
                name="targetNamespace"
                help="留空则使用备份时的命名空间；集群级资源无需填写"
              >
                <Input placeholder="命名空间（可留空）" allowClear />
              </Form.Item>
              <Form.Item
                label="恢复模式"
                name="mode"
                rules={[{ required: true, message: '请选择恢复模式' }]}
                help="覆盖：更新已存在的同名资源（保留 resourceVersion）；新建：资源已存在时将报错"
              >
                <Select
                  placeholder="请选择恢复模式"
                  options={[
                    { label: '新建（create-new）', value: 'create-new' },
                    { label: '覆盖（overwrite）', value: 'overwrite' },
                  ]}
                />
              </Form.Item>
            </Form>
          </Space>
        )}
      </Modal>
    </PageContainer>
  );
};

export default BackupList;
