import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Col,
  Descriptions,
  Drawer,
  Empty,
  List,
  Modal,
  Popconfirm,
  Row,
  Space,
  Tabs,
  Tag,
  Timeline,
  Tooltip,
  message,
} from 'antd';
import { PageContainer } from '@ant-design/pro-components';
import Editor, { DiffEditor } from '@monaco-editor/react';
import type { editor } from 'monaco-editor';
import { dump as yamlDump, load as yamlLoad } from 'js-yaml';
import {
  ArrowLeftOutlined,
  DeleteOutlined,
  HistoryOutlined,
  RollbackOutlined,
  SaveOutlined,
  FileSearchOutlined,
} from '@ant-design/icons';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import dayjs from 'dayjs';
import {
  KubernetesResource,
  useDeleteResourceMutation,
  useGetResourceQuery,
  useUpdateResourceMutation,
} from '@/app/services/resource';
import {
  ResourceVersion,
  useLazyGetVersionDiffQuery,
  useListVersionsQuery,
  useRollbackVersionMutation,
} from '@/app/services/version';
import { useAppDispatch } from '@/app/store';
import { setSelectedClusterCode } from '@/slices/appSlice';
import { usePermission } from '@/hooks/usePermission';
// 本地 Monaco 加载配置（替代 CDN，离线环境可用）
import '@/app/monaco';

interface EditPageParams {
  code: string;
  kind: string;
  namespace: string;
  name: string;
  apiVersion?: string;
}

const yamlStringify = (obj: unknown): string => {
  if (typeof obj === 'string') return obj;
  try {
    return yamlDump(obj);
  } catch {
    return JSON.stringify(obj, null, 2);
  }
};

const tryParseYaml = (text: string): unknown | null => {
  try {
    return yamlLoad(text);
  } catch {
    return null;
  }
};

const ResourceEdit: React.FC = () => {
  const params = useParams<keyof EditPageParams>();
  const navigate = useNavigate();
  const dispatch = useAppDispatch();
  const [searchParams, setSearchParams] = useSearchParams();

  const code = decodeURIComponent(params.code || '');
  const apiVersionPath = decodeURIComponent(
    (params as Record<string, string>).apiVersion || 'apps%2Fv1',
  );
  const kind = decodeURIComponent(params.kind || '');
  const namespaceRaw = decodeURIComponent(params.namespace || '_');
  const namespace = namespaceRaw === '_' ? '' : namespaceRaw;
  const name = decodeURIComponent(params.name || '');

  const [activeTab, setActiveTab] = useState<string>(
    searchParams.get('tab') || 'yaml',
  );

  useEffect(() => {
    if (code) dispatch(setSelectedClusterCode(code));
  }, [code, dispatch]);

  useEffect(() => {
    setSearchParams({ tab: activeTab }, { replace: true });
  }, [activeTab, setSearchParams]);

  const {
    data: resource,
    refetch: refetchResource,
    isFetching: resourceLoading,
  } = useGetResourceQuery(
    { code, apiVersion: apiVersionPath, kind, namespace, name },
    { skip: !code || !kind || !name },
  );

  const { hasPerm } = usePermission();
  const canUpdate = hasPerm('resource:update');
  const canDelete = hasPerm('resource:delete');
  const canRollback = hasPerm('version:rollback') || canUpdate;
  const [yamlValue, setYamlValue] = useState<string>('');
  const [originalYaml, setOriginalYaml] = useState<string>('');
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);

  useEffect(() => {
    if (resource) {
      const y = yamlStringify(resource);
      setYamlValue(y);
      setOriginalYaml(y);
    }
  }, [resource]);

  const handleEditorMount = (ed: editor.IStandaloneCodeEditor) => {
    editorRef.current = ed;
  };

  const handleEditorWillMount = useCallback(
    (monaco: typeof import('monaco-editor')) => {
      (monaco.languages as unknown as {
        yaml?: {
          yamlDefaults?: {
            setDiagnosticsOptions?: (opts: { validate: boolean; enableSchemaRequest: boolean; hover: boolean; completion: boolean; schemas: unknown[] }) => void;
          };
        };
      }).yaml?.yamlDefaults?.setDiagnosticsOptions?.({
        validate: true,
        enableSchemaRequest: false,
        hover: true,
        completion: true,
        schemas: [],
      });
      monaco.editor.defineTheme('kube-yaml', {
        base: 'vs',
        inherit: true,
        rules: [],
        colors: {},
      });
    },
    [],
  );

  const [updateResource, { isLoading: updateLoading }] = useUpdateResourceMutation();
  const [deleteResource] = useDeleteResourceMutation();
  const [rollbackVersion, { isLoading: rollbackLoading }] = useRollbackVersionMutation();
  const [getVersionDiff, { isFetching: diffLoading }] = useLazyGetVersionDiffQuery();

  const { data: versionsData, refetch: refetchVersions } = useListVersionsQuery(
    { code, apiVersion: apiVersionPath, kind, namespace, name },
    { skip: !code || !kind || !name, refetchOnMountOrArgChange: true },
  );
  const versionItems = useMemo(
    () => versionsData?.items?.slice(0, 5) || [],
    [versionsData],
  );

  const isDirty = yamlValue !== originalYaml;

  const handleSave = async () => {
    if (!canUpdate) {
      message.error('无编辑权限');
      return;
    }
    try {
      if (!yamlValue.trim()) {
        message.error('YAML 内容不能为空');
        return;
      }
      const parsed = tryParseYaml(yamlValue);
      const yamlToSend =
        parsed && typeof parsed === 'object' ? yamlStringify(parsed) : yamlValue;
      await updateResource({
        code,
        apiVersion: apiVersionPath,
        kind,
        namespace,
        name,
        yaml: yamlToSend,
      }).unwrap();
      message.success('保存成功');
      await Promise.all([refetchResource(), refetchVersions()]);
    } catch {
      // interceptor handles errors
    }
  };

  const handleDelete = async () => {
    if (!canDelete) {
      message.error('无删除权限');
      return;
    }
    try {
      await deleteResource({
        code,
        apiVersion: apiVersionPath,
        kind,
        namespace,
        name,
      }).unwrap();
      message.success('删除成功');
      navigate(`/resources/list?cluster_code=${encodeURIComponent(code)}`);
    } catch {
      // interceptor
    }
  };

  // version compare
  const [compareDrawerOpen, setCompareDrawerOpen] = useState(false);
  const [checkedSeqs, setCheckedSeqs] = useState<number[]>([]);
  const [diffLeft, setDiffLeft] = useState<string>('');
  const [diffRight, setDiffRight] = useState<string>('');
  const [diffLoaded, setDiffLoaded] = useState(false);

  const toggleSeq = (seq: number) => {
    setCheckedSeqs((prev) => {
      if (prev.includes(seq)) return prev.filter((x) => x !== seq);
      const next = [...prev, seq];
      return next.length > 2 ? next.slice(-2) : next;
    });
    setDiffLoaded(false);
  };

  const openCompare = () => {
    if (checkedSeqs.length !== 2) {
      message.warning('请选择两个版本进行对比');
      return;
    }
    setCompareDrawerOpen(true);
    setDiffLoaded(false);
    setDiffLeft('');
    setDiffRight('');
    const [a, b] = [...checkedSeqs].sort((x, y) => x - y);
    getVersionDiff({
      code,
      apiVersion: apiVersionPath,
      kind,
      namespace,
      name,
      seqA: a,
      seqB: b,
    })
      .unwrap()
      .then((res) => {
        setDiffLeft(res.yaml_a || '');
        setDiffRight(res.yaml_b || '');
        setDiffLoaded(true);
      })
      .catch(() => {
        setDiffLeft(`# 无法加载版本 #${a} 的 YAML`);
        setDiffRight(`# 无法加载版本 #${b} 的 YAML`);
        setDiffLoaded(true);
      });
  };

  const handleRollback = async (seq: number) => {
    if (!canRollback) {
      message.error('无回滚权限');
      return;
    }
    Modal.confirm({
      title: `回滚到版本 #${seq}`,
      content: '将用该历史版本的 YAML 覆盖当前集群中的对象，是否继续？',
      okButtonProps: { danger: true },
      okText: '确认回滚',
      onOk: async () => {
        try {
          const res = await rollbackVersion({
            code,
            apiVersion: apiVersionPath,
            kind,
            namespace,
            name,
            seq,
          }).unwrap();
          message.success(res.rolled_back ? `已回滚到版本 #${res.seq}` : '回滚成功');
          await Promise.all([refetchResource(), refetchVersions()]);
        } catch {
          // interceptor
        }
      },
    });
  };

  const metadata: KubernetesResource['metadata'] | undefined = resource?.metadata;
  const creationTs = metadata?.creationTimestamp;

  return (
    <PageContainer
      onBack={() => navigate(-1)}
      backIcon={<ArrowLeftOutlined />}
      title={
        <Space>
          <span>
            {kind} / {name}
          </span>
          <Tag color="blue">集群 {code}</Tag>
          {namespace && <Tag>{namespace}</Tag>}
        </Space>
      }
      extra={
        <Space>
          {canUpdate && (
            <Button
              type="primary"
              icon={<SaveOutlined />}
              onClick={handleSave}
              loading={updateLoading}
              disabled={!isDirty}
            >
              保存{isDirty ? '（有未保存修改）' : ''}
            </Button>
          )}
          {canDelete && (
            <Popconfirm
              title={`确认删除当前 ${kind}「${name}」？`}
              okButtonProps={{ danger: true }}
              description="该操作会从集群中移除该对象"
              onConfirm={handleDelete}
            >
              <Button danger icon={<DeleteOutlined />}>
                删除
              </Button>
            </Popconfirm>
          )}
        </Space>
      }
    >
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        {isDirty && (
          <Alert
            type="warning"
            showIcon
            message="YAML 已被修改但尚未保存，点击右上角【保存】将更新写入集群。"
          />
        )}

        <Row gutter={[16, 16]}>
          <Col xs={24} xl={17}>
            <Card
              title={
                <Space>
                  <FileSearchOutlined />
                  <span>YAML 编辑器</span>
                </Space>
              }
              bordered={false}
              style={{ height: 'calc(100vh - 300px)', minHeight: 520 }}
              bodyStyle={{ height: 'calc(100% - 56px)', padding: 0 }}
              extra={
                <Space>
                  <Tag color={isDirty ? 'warning' : 'green'}>
                    {isDirty ? '未保存' : '已同步'}
                  </Tag>
                </Space>
              }
            >
              <Tabs
                activeKey={activeTab}
                onChange={setActiveTab}
                size="small"
                style={{ height: '100%' }}
                items={[
                  {
                    key: 'yaml',
                    label: 'YAML',
                    children: (
                      <div style={{ height: 'calc(100% - 44px)' }}>
                        {resourceLoading && !resource ? (
                          <Empty description="加载中..." />
                        ) : (
                          <Editor
                            height="100%"
                            defaultLanguage="yaml"
                            language="yaml"
                            theme="kube-yaml"
                            value={yamlValue}
                            onChange={(v) => setYamlValue(v || '')}
                            onMount={handleEditorMount}
                            beforeMount={handleEditorWillMount}
                            loading={<Empty description="加载编辑器..." />}
                            options={{
                              minimap: { enabled: false },
                              fontSize: 13,
                              lineNumbers: 'on',
                              automaticLayout: true,
                              scrollBeyondLastLine: false,
                              renderWhitespace: 'boundary',
                              tabSize: 2,
                              insertSpaces: true,
                              wordWrap: 'on',
                              readOnly: !canUpdate,
                            }}
                          />
                        )}
                      </div>
                    ),
                  },
                  {
                    key: 'versions',
                    label: (
                      <Space>
                        <HistoryOutlined />
                        版本历史
                      </Space>
                    ),
                    children: (
                      <div style={{ height: 'calc(100% - 44px)', overflow: 'auto', padding: 16 }}>
                        {versionItems.length === 0 ? (
                          <Empty description="暂无历史版本" />
                        ) : (
                          <Space direction="vertical" style={{ width: '100%' }} size="large">
                            <Space>
                              <Tooltip title="请选择两个版本进行对比">
                                <Button
                                  icon={<FileSearchOutlined />}
                                  disabled={checkedSeqs.length !== 2}
                                  onClick={openCompare}
                                >
                                  对比已选 {checkedSeqs.length}/2
                                </Button>
                              </Tooltip>
                              <span style={{ color: 'rgba(0,0,0,0.45)' }}>
                                勾选左侧复选框选择两个版本
                              </span>
                            </Space>

                            <Timeline
                              mode="left"
                              items={versionItems.map((v: ResourceVersion) => ({
                                color:
                                  v.source === 'rollback'
                                    ? 'orange'
                                    : v.source === 'backup'
                                      ? 'purple'
                                      : 'blue',
                                children: (
                                  <Card
                                    size="small"
                                    title={
                                      <Space>
                                        <Checkbox
                                          checked={checkedSeqs.includes(v.version_seq)}
                                          onChange={() => toggleSeq(v.version_seq)}
                                        >
                                          <strong># {v.version_seq}</strong>
                                        </Checkbox>
                                        <Tag>{v.change_summary || v.source || 'snapshot'}</Tag>
                                      </Space>
                                    }
                                    extra={
                                      <Space>
                                        <span style={{ color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>
                                          {v.created_at
                                            ? dayjs(v.created_at).format('YYYY-MM-DD HH:mm:ss')
                                            : ''}
                                        </span>
                                        {canRollback && (
                                        <Button
                                          type="link"
                                          size="small"
                                          icon={<RollbackOutlined />}
                                          loading={rollbackLoading}
                                          onClick={() => handleRollback(v.version_seq)}
                                        >
                                          回滚到此版本
                                        </Button>
                                        )}
                                      </Space>
                                    }
                                  >
                                    <Descriptions size="small" column={1} bordered>
                                      <Descriptions.Item label="操作人">
                                        {v.operator || '-'}
                                      </Descriptions.Item>
                                    </Descriptions>
                                  </Card>
                                ),
                              }))}
                            />
                          </Space>
                        )}
                      </div>
                    ),
                  },
                ]}
              />
            </Card>
          </Col>

          <Col xs={24} xl={7}>
            <Card
              title={
                <Space>
                  <FileSearchOutlined />
                  摘要
                </Space>
              }
              bordered={false}
              loading={resourceLoading && !resource}
            >
              <Descriptions column={1} size="small" bordered>
                <Descriptions.Item label="Kind">
                  <Tag color="blue">{kind}</Tag>
                </Descriptions.Item>
                <Descriptions.Item label="Name">{metadata?.name || '-'}</Descriptions.Item>
                <Descriptions.Item label="Namespace">
                  {namespace || <Tag>集群级</Tag>}
                </Descriptions.Item>
                <Descriptions.Item label="UID">
                  <span style={{ fontFamily: 'monospace', fontSize: 12 }}>
                    {metadata?.uid || '-'}
                  </span>
                </Descriptions.Item>
                <Descriptions.Item label="ResourceVersion">
                  {metadata?.resourceVersion || '-'}
                </Descriptions.Item>
                <Descriptions.Item label="创建时间">
                  {creationTs
                    ? `${dayjs(creationTs).fromNow()} (${dayjs(creationTs).format(
                        'YYYY-MM-DD HH:mm:ss',
                      )})`
                    : '-'}
                </Descriptions.Item>
                <Descriptions.Item label="Labels">
                  {metadata?.labels && Object.keys(metadata.labels).length > 0 ? (
                    <List
                      size="small"
                      dataSource={Object.entries(metadata.labels)}
                      renderItem={([k, v]) => (
                        <List.Item>
                          <Tag>{k}={v ? `=${v}` : ''}</Tag>
                        </List.Item>
                      )}
                    />
                  ) : (
                    '-'
                  )}
                </Descriptions.Item>
                <Descriptions.Item label="Annotations">
                  {metadata?.annotations && Object.keys(metadata.annotations).length > 0 ? (
                    <List
                      size="small"
                      dataSource={Object.entries(metadata.annotations).slice(0, 5)}
                      renderItem={([k, v]) => (
                        <List.Item>
                          <Space direction="vertical" size={0} style={{ width: '100%' }}>
                            <code style={{ fontSize: 11, wordBreak: 'break-all' }}>{k}</code>
                            <span
                              style={{
                                fontSize: 12,
                                color: 'rgba(0,0,0,0.7)',
                                wordBreak: 'break-all',
                              }}
                            >
                              {String(v).slice(0, 120)}
                              {String(v).length > 120 ? '...' : ''}
                            </span>
                          </Space>
                        </List.Item>
                      )}
                    />
                  ) : (
                    '-'
                  )}
                </Descriptions.Item>
              </Descriptions>
            </Card>
          </Col>
        </Row>
      </Space>

      <Drawer
        title={`版本对比 ${
          checkedSeqs.length === 2
            ? `#${Math.min(...checkedSeqs)} ↔ #${Math.max(...checkedSeqs)}`
            : ''
        }`}
        width="85%"
        open={compareDrawerOpen}
        onClose={() => setCompareDrawerOpen(false)}
        destroyOnClose
      >
        {!diffLoaded || diffLoading ? (
          <Empty description="加载 Diff 中..." />
        ) : (
          <div style={{ height: 'calc(100vh - 200px)' }}>
            <DiffEditor
              height="100%"
              original={diffLeft}
              modified={diffRight}
              originalLanguage="yaml"
              modifiedLanguage="yaml"
              theme="kube-yaml"
              beforeMount={handleEditorWillMount}
              options={{
                minimap: { enabled: false },
                fontSize: 13,
                readOnly: true,
                automaticLayout: true,
                renderSideBySide: true,
                wordWrap: 'on',
              }}
            />
          </div>
        )}
      </Drawer>
    </PageContainer>
  );
};

export default ResourceEdit;
