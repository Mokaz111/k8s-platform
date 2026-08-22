import React from 'react';
import { Tabs } from 'antd';
import { PageContainer } from '@ant-design/pro-components';
import { useParams } from 'react-router-dom';

const Workloads: React.FC = () => {
  const { type } = useParams();
  const [activeKey, setActiveKey] = React.useState(type || 'deployments');

  return (
    <PageContainer title="工作负载">
      <Tabs
        activeKey={activeKey}
        onChange={setActiveKey}
        items={[
          { key: 'deployments', label: 'Deployments' },
          { key: 'statefulsets', label: 'StatefulSets' },
          { key: 'daemonsets', label: 'DaemonSets' },
          { key: 'jobs', label: 'Jobs' },
          { key: 'cronjobs', label: 'CronJobs' },
          { key: 'pods', label: 'Pods' },
        ]}
      />
    </PageContainer>
  );
};

export default Workloads;
