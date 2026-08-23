import React, { useRef, useState } from 'react';
import {
  Button,
  Card,
  Form,
  Input,
  Radio,
  Space,
  Spin,
  Upload,
  message,
} from 'antd';
import {
  PageContainer,
  ProForm,
  ProFormItem,
  ProFormText,
  ProFormTextArea,
} from '@ant-design/pro-components';
import {
  ArrowLeftOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExperimentOutlined,
  ImportOutlined,
  UploadOutlined,
} from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import type { UploadFile, UploadProps } from 'antd/es/upload/interface';
import { RcFile } from 'antd/es/upload';
import {
  useImportClusterMutation,
  useTempPingMutation,
} from '@/app/services/cluster';

type KubeconfigMode = 'file' | 'text';

interface ImportFormValues {
  name: string;
  code: string;
  description?: string;
  kubeconfig?: string;
}

const ClusterImport: React.FC = () => {
  const navigate = useNavigate();
  const [form] = Form.useForm<ImportFormValues>();
  const [mode, setMode] = useState<KubeconfigMode>('text');
  const [fileList, setFileList] = useState<UploadFile[]>([]);
  const [pingLoading, setPingLoading] = useState(false);
  const [pingResult, setPingResult] = useState<{
    success?: boolean;
    message?: string;
  } | null>(null);
  const fileContentRef = useRef<string>('');

  const [importCluster, { isLoading: importLoading }] = useImportClusterMutation();
  const [tempPing] = useTempPingMutation();

  const extractKubeconfig = async (values: ImportFormValues): Promise<string | undefined> => {
    if (mode === 'text') {
      return values.kubeconfig;
    }
    return fileContentRef.current || undefined;
  };

  const readFileAsText = (file: RcFile): Promise<string> =>
    new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result || ''));
      reader.onerror = reject;
      reader.readAsText(file);
    });

  const beforeUpload: UploadProps['beforeUpload'] = async (file) => {
    const allowed = ['.yaml', '.yml', '.kubeconfig'];
    const fn = file.name.toLowerCase();
    if (!allowed.some((ext) => fn.endsWith(ext))) {
      message.error('仅支持 .yaml/.yml/.kubeconfig 文件');
      return Upload.LIST_IGNORE;
    }
    try {
      const text = await readFileAsText(file);
      fileContentRef.current = text;
      form.setFieldsValue({ kubeconfig: text });
      setFileList([{ ...file, status: 'done', uid: file.uid }]);
    } catch {
      message.error('读取文件失败');
      return Upload.LIST_IGNORE;
    }
    return false;
  };

  const handleTestConnection = async () => {
    try {
      const values = await form.validateFields(['name', 'code']);
      const kc = await extractKubeconfig(values);
      if (!kc) {
        message.error('请提供 Kubeconfig 内容');
        return;
      }
      setPingLoading(true);
      setPingResult(null);
      const res = await tempPing({ kubeconfig_text: kc }).unwrap();
      setPingResult({ success: res.success, message: res.message });
      if (res.success) {
        message.success(res.message || '连接成功');
      } else {
        message.error(res.message || '连接失败');
      }
    } catch {
      setPingResult({ success: false, message: '连接请求失败' });
    } finally {
      setPingLoading(false);
    }
  };

  const handleSubmit = async (values: ImportFormValues) => {
    try {
      const kc = await extractKubeconfig(values);
      if (!kc) {
        message.error('请提供 Kubeconfig 内容');
        return;
      }
      await importCluster({
        name: values.name,
        code: values.code,
        description: values.description,
        kubeconfig: kc,
      }).unwrap();
      message.success('集群导入成功');
      navigate('/clusters/list');
    } catch {
      // error handled by interceptor
    }
  };

  return (
    <PageContainer
      onBack={() => navigate('/clusters/list')}
      backIcon={<ArrowLeftOutlined />}
      title="导入集群"
    >
      <Card bordered={false}>
        <ProForm<ImportFormValues>
          form={form}
          layout="vertical"
          onFinish={handleSubmit}
          submitter={{
            render: (_props, doms) => {
              return [
                <Space key="actions" wrap>
                  <Button onClick={() => navigate('/clusters/list')}>返回</Button>
                  <Button
                    icon={<ExperimentOutlined />}
                    loading={pingLoading}
                    onClick={handleTestConnection}
                  >
                    测试连接
                  </Button>
                  {pingResult && (
                    <Space>
                      {pingResult.success ? (
                        <span style={{ color: '#52c41a' }}>
                          <CheckCircleOutlined /> 连接成功
                          {pingResult.message ? `：${pingResult.message}` : ''}
                        </span>
                      ) : (
                        <span style={{ color: '#ff4d4f' }}>
                          <CloseCircleOutlined /> 连接失败
                          {pingResult.message ? `：${pingResult.message}` : ''}
                        </span>
                      )}
                    </Space>
                  )}
                  {doms[1]}
                </Space>,
              ];
            },
            searchConfig: {
              submitText: '导入',
              resetText: '重置',
            },
            submitButtonProps: {
              loading: importLoading,
              icon: <ImportOutlined />,
              type: 'primary',
            },
          }}
        >
          <ProFormText
            name="name"
            label="集群名称"
            placeholder="请输入集群显示名称"
            rules={[{ required: true, message: '请输入集群名称' }]}
            fieldProps={{ maxLength: 64, showCount: true }}
          />
          <ProFormText
            name="code"
            label="集群编码"
            placeholder="唯一标识，建议英文/数字/短横线（例如 prod-cluster-01）"
            rules={[
              { required: true, message: '请输入集群编码' },
              {
                pattern: /^[a-zA-Z0-9][a-zA-Z0-9_-]*$/,
                message: '编码只能包含字母、数字、短横线或下划线',
              },
              { max: 64, message: '长度不能超过 64' },
            ]}
            fieldProps={{ maxLength: 64, showCount: true }}
          />
          <ProFormTextArea
            name="description"
            label="描述"
            placeholder="可选，说明此集群的用途"
            fieldProps={{ rows: 3, maxLength: 256, showCount: true }}
          />

          <ProFormItem label="Kubeconfig" required>
            <Space direction="vertical" style={{ width: '100%' }} size="middle">
              <Radio.Group
                value={mode}
                onChange={(e) => setMode(e.target.value as KubeconfigMode)}
                optionType="button"
                buttonStyle="solid"
              >
                <Radio.Button value="text">粘贴内容</Radio.Button>
                <Radio.Button value="file">上传文件</Radio.Button>
              </Radio.Group>

              {mode === 'text' ? (
                <Form.Item
                  name="kubeconfig"
                  noStyle
                  rules={[{ required: true, message: '请粘贴 Kubeconfig 内容' }]}
                >
                  <Input.TextArea
                    rows={14}
                    placeholder="请粘贴 Kubeconfig YAML 内容"
                    style={{ fontFamily: 'monospace' }}
                    onChange={() => setPingResult(null)}
                  />
                </Form.Item>
              ) : (
                <Space direction="vertical" style={{ width: '100%' }} size="middle">
                  <Upload
                    listType="text"
                    maxCount={1}
                    accept=".yaml,.yml,.kubeconfig"
                    fileList={fileList}
                    beforeUpload={beforeUpload}
                    onRemove={() => {
                      setFileList([]);
                      fileContentRef.current = '';
                      form.setFieldsValue({ kubeconfig: '' });
                      setPingResult(null);
                    }}
                    onChange={({ fileList: list }) => setFileList(list)}
                  >
                    <Button icon={<UploadOutlined />}>
                      选择 .yaml / .yml / .kubeconfig 文件
                    </Button>
                  </Upload>
                  {pingLoading && <Spin />}
                </Space>
              )}
            </Space>
          </ProFormItem>
        </ProForm>
      </Card>
    </PageContainer>
  );
};

export default ClusterImport;
