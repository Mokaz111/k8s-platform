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
  BackupMode,
  BackupStatus,
  StorageType,
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

const modeLabelMap: Record<BackupMode | string, string> = {
  single: '单对象',
  namespace_batch: '命名空间批量',
  object: '单对象',
  namespace: '命名空间级',
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
  const { hasPerm } = usePermission();
  const canView = hasPerm('backup:view') || hasPerm('backup:list') || hasPerm('backup:get');
  const canCreate = hasPerm('backup:create');
  const canRestore = hasPerm('backup:restore');
  const canDownload = hasPerm('backup:download');
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
      targetCluster: record.code,
      targetNamespace: record.namespace,
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
        body: values,
      }).unwrap();
      const target = values.targetCluster || restoringRecord.code;
      const tip = res.taskId
        ? `恢复任务 #${res.taskId} 已提交，切到集群 ${target} 查看进度`
        : res.message || '恢复任务已提交';
      message.success(tip);
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
    try {
      await downloadBackupById(record);
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
        render: (_dom, record) => (
          <Space direction="vertical" size={0}>
            <span>{record.createdAt ? dayjs(record.createdAt).format('YYYY-MM-DD HH:mm:ss') : '-'}</span>
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
        render: (_dom, record) => record.operator || '-',
      },
      {
        title: '集群',
        dataIndex: 'code',
        key: 'code',
        width: 140,
        render: (_dom, record) => (
          <Space>
            <Tag color="blue">{record.code}</Tag>
            {record.clusterName && (
              <span style={{ color: 'rgba(0,0,0,0.65)' }}>{record.clusterName}</span>
            )}
          </Space>
        ),
      },
      {
        title: '类型',
        key: 'taskType',
        width: 100,
        render: (_dom, record) => {
          const isRestore = record.backupType === 'restore';
          return (
            <Tag color={isRestore ? 'orange' : 'blue'}>
              {isRestore ? '恢复' : '备份'}
            </Tag>
          );
        },
      },
      {
        title: '备份模式',
        dataIndex: 'mode',
        key: 'mode',
        width: 120,
        render: (_dom, record) => {
          const displayMode = record.mode || (record.namespaces && record.namespaces.length > 1 ? 'namespace_batch' : 'single');
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
          const batchNs = record.namespaces;
          if (batchNs && batchNs.length > 0) {
            if (batchNs.length <= 3) {
              return (
                <Space size={[4, 4]} wrap>
                  {batchNs.map((n) => (
                    <Tag key={n} color="blue">{n}</Tag>
                  ))}
                </Space>
              );
            }
            return (
              <Space size={[4, 4]} wrap>
                {batchNs.slice(0, 3).map((n) => (
                  <Tag key={n} color="blue">{n}</Tag>
                ))}
                <Tag color="default">+{batchNs.length - 3}</Tag>
              </Space>
            );
          }
          return record.namespace || <Tag>（集群级）</Tag>;
        },
      },
      {
        title: '对象类型 / 名称',
        key: 'object',
        width: 260,
        render: (_v, record) => {
          // 命名空间批量模式：展示 kind_filter 列表
          const kf = record.kindFilter;
          if (kf && kf.length > 0) {
            if (kf.length <= 4) {
              return (
                <Space size={[4, 4]} wrap>
                  {kf.map((k) => (
                    <Tag key={k} color="purple">{k}</Tag>
                  ))}
                </Space>
              );
            }
            return (
              <Space size={[4, 4]} wrap>
                {kf.slice(0, 4).map((k) => (
                  <Tag key={k} color="purple">{k}</Tag>
                ))}
                <Tag color="default">+{kf.length - 4}</Tag>
              </Space>
            );
          }
          // 单对象/命名空间级模式：展示 kind + name 链接
          return (
            <Space direction="vertical" size={0}>
              <Tag color="purple">{record.kind}</Tag>
              {record.name && (
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
              )}
            </Space>
          );
        },
      },
      {
        title: '存储类型',
        dataIndex: 'storageType',
        key: 'storageType',
        width: 110,
        render: (_dom, record) => (
          <Tag color={storageColorMap[record.storageType] || 'default'}>{record.storageType}</Tag>
        ),
      },
      {
        title: '大小',
        dataIndex: 'size',
        key: 'size',
        width: 110,
        render: (_dom, record) => {
          const v = record.size;
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
          const isBatch =
            record.mode === 'namespace_batch' ||
            (record.namespaces && record.namespaces.length > 1) ||
            (record.kindFilter && record.kindFilter.length > 1);
          return (
            <Space size="small" wrap>
              <Button
                type="link"
                size="small"
                icon={<RollbackOutlined />}
                onClick={() => handleOpenRestore(record)}
                disabled={record.status !== 'Success' || !canRestore}
              >
                恢复
              </Button>
              <Button
                type="link"
                size="small"
                icon={<DownloadOutlined />}
                onClick={() => handleDownload(record)}
                disabled={record.status !== 'Success' || !canDownload}
              >
                下载
              </Button>
              <Button
                type="link"
                size="small"
                icon={<HistoryOutlined />}
                disabled={isBatch || !record.name}
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
                title={`确认删除备份「${record.name || `#${record.id}`}」？`}
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
