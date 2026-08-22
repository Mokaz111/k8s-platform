import React from 'react';
import { Card, Col, Row, Statistic } from 'antd';
import {
  ClusterOutlined,
  AppstoreOutlined,
  ApiOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { PageContainer } from '@ant-design/pro-components';
import { useAppDispatch } from '@/app/store';
import { clearClusterEvents } from '@/slices/wsSlice';
import { ClusterEventsList } from '@/components/ws';

const Dashboard: React.FC = () => {
  const dispatch = useAppDispatch();
  return (
    <PageContainer>
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="集群总数"
              value={0}
              prefix={<ClusterOutlined style={{ color: '#1677ff' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="工作负载"
              value={0}
              prefix={<AppstoreOutlined style={{ color: '#52c41a' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="API 调用"
              value={0}
              prefix={<ApiOutlined style={{ color: '#faad14' }} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="活跃用户"
              value={0}
              prefix={<UserOutlined style={{ color: '#eb2f96' }} />}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={12}>
          <Card
            title="实时集群事件"
            bordered={false}
            bodyStyle={{ padding: 0 }}
          >
            <ClusterEventsList
              height={420}
              maxItems={50}
              onClear={() => dispatch(clearClusterEvents())}
            />
          </Card>
        </Col>
      </Row>
    </PageContainer>
  );
};

export default Dashboard;
