import React from 'react';
import { Card, Col, Row, Statistic } from 'antd';
import { ClusterOutlined } from '@ant-design/icons';
import { PageContainer } from '@ant-design/pro-components';
import { useAppDispatch, useAppSelector } from '@/app/store';
import { clearClusterEvents } from '@/slices/wsSlice';
import { ClusterEventsList } from '@/components/ws';
import { useListClustersQuery } from '@/app/services/cluster';

const Dashboard: React.FC = () => {
  const dispatch = useAppDispatch();
  const token = useAppSelector((s) => s.user.token);
  const selectedClusterCode = useAppSelector((s) => s.app.selectedClusterCode);
  const { data, isFetching } = useListClustersQuery(undefined, { skip: !token });
  const clusterTotal = data?.total ?? data?.items?.length ?? 0;

  return (
    <PageContainer>
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="已纳管集群"
              value={clusterTotal}
              loading={isFetching && data === undefined}
              prefix={<ClusterOutlined style={{ color: '#1677ff' }} />}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={12}>
          <Card
            title={selectedClusterCode ? `实时集群事件（${selectedClusterCode}）` : '实时集群事件'}
            bordered={false}
            bodyStyle={{ padding: 0 }}
          >
            <ClusterEventsList
              height={420}
              maxItems={50}
              clusterCode={selectedClusterCode || undefined}
              onClear={() => dispatch(clearClusterEvents())}
            />
          </Card>
        </Col>
      </Row>
    </PageContainer>
  );
};

export default Dashboard;
