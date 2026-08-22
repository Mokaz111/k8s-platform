# K8S 多集群管理平台 — System Design

> 版本：v1.0  
> 目标：设计一个可插件化扩展的 K8S 多集群管理平台，支持导入集群证书后，通过界面化管理多种 K8S 对象，并对关键对象进行备份与版本历史管理。

---

## 1. 产品定位与目标

### 1.1 核心目标
- 将 K8S 集群中的 YAML 资源从“黑盒”变为“白盒”，通过可视化界面降低运维门槛。
- 支持通过导入集群证书（kubeconfig / 证书文件）快速纳管多个 K8S 集群。
- 对工作负载（Deployment、StatefulSet、DaemonSet、Job、CronJob）、ConfigMap、PV、PVC、StorageClass 等对象进行查看、编辑、备份。
- 每次资源修改都在界面完成，并保留最近 5 个版本的历史记录，支持回滚与对比。
- 采用插件式架构，预留后续扩展空间（新对象类型、新云厂商、新备份目标、新审批流程等）。

### 1.2 目标用户
- SRE / 平台工程师：多集群统一管理、批量操作、审计与备份。
- 开发团队：查看/修改自己命名空间下的工作负载与配置。
- 运维主管：版本历史审计、变更回溯、权限管控。

---

## 2. 功能需求

### 2.1 集群管理
| 功能 | 说明 |
|------|------|
| 集群导入 | 支持上传 kubeconfig 文件或粘贴证书内容；解析并加密存储。 |
| 集群列表 | 展示已导入集群名称、版本、节点数、状态、最近同步时间。 |
| 集群切换 | 全局集群上下文切换器，所有资源操作均基于当前选中的集群。 |
| 连接检测 | 导入后自动连通性检测；列表页显示在线/离线状态。 |
| 证书更新 | 支持重新上传证书或修改 API Server 地址。 |

### 2.2 资源对象管理（第一阶段）
| 对象类别 | 支持操作 |
|----------|----------|
| Workload | Deployment、StatefulSet、DaemonSet、ReplicaSet、Job、CronJob 的列表、详情、YAML 编辑、删除。 |
| Config & Secret | ConfigMap、Secret（Opaque、TLS、Registry 等）列表、编辑、版本历史。 |
| Storage | PersistentVolume、PersistentVolumeClaim、StorageClass 列表、详情、YAML 编辑。 |
| Service & Network（预留） | Service、Ingress、NetworkPolicy（插件化实现）。 |
| RBAC（预留） | Role、ClusterRole、RoleBinding（插件化实现）。 |

### 2.3 备份管理
- 对指定资源对象进行单点备份，保存其完整 YAML 快照。
- 支持命名空间级批量备份（插件扩展点）。
- 备份记录列表：备份时间、操作人、对象类型/名称、存储位置、状态。
- 支持从备份恢复（覆盖或新建）。

### 2.4 版本历史
- 每次通过本平台修改对象，自动生成版本快照。
- 每个对象保留最近 5 个版本，更早版本自动清理（可配置）。
- 版本对比：左右对照展示两个版本的 YAML 差异。
- 版本回滚：将对象恢复至选定的历史版本（生成新版本记录）。

### 2.5 插件式扩展
- 插件注册中心：定义插件元数据、能力声明、依赖校验。
- 对象类型插件：新增 K8S API Group/Kind 支持。
- 备份目标插件：本地存储、S3、NFS 等。
- 认证/审计/通知插件：SSO、操作审计、变更通知。

---

## 3. 非功能需求

| 维度 | 要求 |
|------|------|
| 安全性 | 集群证书加密存储（AES-256-GCM / 硬件 KMS）；权限最小化；操作审计日志。 |
| 可靠性 | 连接池与超时控制；K8S API 异常降级；关键操作幂等。 |
| 性能 | 列表分页与缓存；Informer 本地缓存减少 API Server 压力。 |
| 可扩展性 | 插件化对象管理器、备份处理器、通知通道。 |
| 可维护性 | 前后端分层清晰；统一错误码；模块接口化。 |
| 兼容性 | 支持 K8S 1.20+；适配不同云厂商与私有化部署。 |

---

## 4. 系统架构

### 4.1 总体架构（API Server / Task Worker 分离部署）

```
┌──────────────────────────────────────────────────────────────────────────┐
│                           前端 (Web UI / Ant Design Pro)                   │
│  集群总览 │ 资源列表 │ YAML 编辑器 │ 版本历史 │ 备份管理 │ 插件中心        │
│  WebSocket: Pod日志流 · 任务进度 · 集群事件                                │
└────────────────────────────────┬─────────────────────────────────────────┘
                                 │ HTTPS / WebSocket
┌────────────────────────────────▼─────────────────────────────────────────┐
│                       API Server (Go + Gin)                               │
│  ┌─────────────┬──────────────┬──────────────┬─────────────────────┐     │
│  │ Auth (JWT)  │ 请求路由      │ 限流熔断     │ 审计中间件          │     │
│  │ RBAC 校验   │ RESTful API  │ 参数校验     │ 操作日志            │     │
│  └──────┬──────┴──────┬───────┴──────┬───────┴───────────┬─────────┘     │
│         │             │              │                   │               │
│  ┌──────▼──────┐ ┌────▼─────┐ ┌──────▼──────┐ ┌────────▼─────────┐     │
│  │ Cluster     │ │ Resource │ │   Version   │ │  Backup Manager  │     │
│  │ Manager     │ │ Manager  │ │   Manager   │ │ (仅提交任务)      │     │
│  │ 连接池      │ │Informer缓存│ │ 版本快照     │ └────────┬─────────┘     │
│  │ 健康检测    │ │ Lazy Load │ │ diff/回滚    │          │               │
│  └──────┬──────┘ └────┬─────┘ └──────┬──────┘          │               │
│         │             │              │                   │               │
│  ┌──────▼─────────────▼──────────────▼───────────────────▼──────────┐   │
│  │                    Plugin Manager (go-plugin / gRPC)              │   │
│  │         IResourceHandler · IBackupStorage · IAuthProvider         │   │
│  │         INotifier · IAuditExporter · IKMSProvider                 │   │
│  └───────────────────────────────┬───────────────────────────────────┘   │
└──────────────────────────────────┼───────────────────────────────────────┘
                                   │
       ┌───────────────────────────┼───────────────────────────┐
       │ Redis Stream              │                           │
       │ (任务队列 / 分布式锁)      │                           │
       └───────────────────────────┬───────────────────────────┘
                                   │
┌──────────────────────────────────▼───────────────────────────────────────┐
│                    Task Worker (独立进程 / 可水平扩缩容)                   │
│  ┌──────────────────────────────┐   ┌───────────────────────────────┐    │
│  │ Backup Worker (消费队列)      │   │ Informer Sync Worker          │    │
│  │ · 单对象备份/恢复             │   │ · 按需启动/停止 Informer       │    │
│  │ · 命名空间批量备份            │   │ · 带限流 List / Watch          │    │
│  │ · S3 / NFS / Local 存储适配   │   │ · 缓存状态同步到 Redis          │    │
│  └──────────────────────────────┘   └───────────────────────────────┘    │
│  ┌──────────────────────────────┐   ┌───────────────────────────────┐    │
│  │ Cluster Health Worker        │   │ WebSocket Notifier            │    │
│  │ · 周期性连通性检测            │   │ · 任务进度推送                 │    │
│  │ · 版本/节点数更新             │   │ · 集群事件/状态广播             │    │
│  └──────────────────────────────┘   └───────────────────────────────┘    │
└──────────────────────────────────┬───────────────────────────────────────┘
                                   │
┌──────────────────────────────────▼───────────────────────────────────────┐
│                              数据层                                       │
│  PostgreSQL: 集群 · 版本快照 · 备份任务 · 插件注册 · RBAC · 审计日志      │
│  Redis:      Informer 状态 · 分布式锁 · WebSocket 会话 · 限流计数器      │
│  Backup:     本地磁盘 (MVP) · S3 兼容 (MinIO/OSS/S3) · NFS               │
└──────────────────────────────────────────────────────────────────────────┘
```

### 4.2 模块职责

| 模块 | 所在进程 | 职责 |
|------|----------|------|
| Cluster Manager | API Server | 集群 CRUD、kubeconfig 加密存储、Dynamic Client 连接池维护、健康检测触发、上下文分发。 |
| Resource Manager | API Server | 抽象 K8S 对象操作接口；加载 ResourcePlugin；按需懒启动 Informer 本地缓存；调用 Dynamic Client 执行增删改查。 |
| Version Manager | API Server | 对象修改前快照、版本链维护（保留最近 N 条）、YAML diff、回滚逻辑编排。 |
| Backup Manager (API侧) | API Server | 备份/恢复任务参数校验、提交到 Redis Stream、任务状态查询。 |
| Auth & RBAC | API Server | 用户登录、JWT 签发、权限点校验、角色与用户绑定管理。 |
| Plugin Manager | API Server + Worker | go-plugin(gRPC) 子进程生命周期管理、能力注册、依赖校验、插件热插拔预留；统一代理调用各插件接口。 |
| Backup Worker | Task Worker | 消费备份/恢复任务、读取 K8S 资源 YAML、通过 IBackupStorage 写入/读取存储后端（Local/S3/NFS）、更新任务状态与进度。 |
| Informer Sync Worker | Task Worker | 按需启动/停止指定集群的 Informer（Lazy Load）、List 阶段带 QPS/Burst 限流、Watch 断线自动重连、同步缓存元数据到 Redis。 |
| Cluster Health Worker | Task Worker | 周期性对所有集群执行连通性检测、更新 version/node_count/status/last_sync_at。 |
| WebSocket Notifier | API Server + Worker | 订阅 Redis Pub/Sub，向在线前端会话推送：备份任务进度、集群状态变化、K8S Event 事件、Pod 日志流。 |


---

## 5. 数据模型

### 5.1 核心实体

#### Cluster（集群）
```sql
CREATE TABLE cluster (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(64) NOT NULL UNIQUE COMMENT '显示名称',
    code VARCHAR(64) NOT NULL UNIQUE COMMENT '唯一标识',
    kubeconfig TEXT NOT NULL COMMENT '加密后的 kubeconfig',
    api_server VARCHAR(256) COMMENT 'API Server 地址',
    status TINYINT DEFAULT 0 COMMENT '0 离线 1 在线',
    version VARCHAR(32) COMMENT 'K8S 版本',
    node_count INT DEFAULT 0,
    last_sync_at TIMESTAMP NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

#### ResourceSnapshot（资源版本快照）
```sql
CREATE TABLE resource_snapshot (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    cluster_code VARCHAR(64) NOT NULL,
    namespace VARCHAR(64) NOT NULL DEFAULT '_cluster_',
    api_version VARCHAR(128) NOT NULL,
    kind VARCHAR(64) NOT NULL,
    name VARCHAR(256) NOT NULL,
    version_seq INT NOT NULL COMMENT '版本序号，倒序',
    raw_yaml MEDIUMTEXT NOT NULL COMMENT '完整 YAML',
    change_summary VARCHAR(512) COMMENT '变更摘要',
    operator VARCHAR(64) NOT NULL,
    source VARCHAR(32) DEFAULT 'ui' COMMENT 'ui / rollback / backup',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_cluster_kind_name (cluster_code, api_version, kind, name),
    INDEX idx_version_seq (cluster_code, namespace, kind, name, version_seq)
);
```

#### BackupTask（备份任务）
```sql
CREATE TABLE backup_task (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    cluster_code VARCHAR(64) NOT NULL,
    namespace VARCHAR(64),
    target_kind VARCHAR(64),
    target_name VARCHAR(256),
    backup_type ENUM('single', 'namespace') NOT NULL,
    storage_type VARCHAR(32) DEFAULT 'local' COMMENT 'local / s3 / nfs',
    storage_path VARCHAR(512) COMMENT '存储路径或 key',
    status VARCHAR(32) DEFAULT 'running',
    size_bytes BIGINT,
    operator VARCHAR(64),
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP NULL
);
```

#### PluginRegistry（插件注册）
```sql
CREATE TABLE plugin_registry (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    plugin_id VARCHAR(64) NOT NULL UNIQUE COMMENT '插件唯一标识',
    name VARCHAR(128) NOT NULL,
    version VARCHAR(32) NOT NULL,
    kind VARCHAR(32) COMMENT 'resource / backup_storage / auth / notifier / audit / kms',
    capabilities JSON COMMENT '能力声明',
    entry_path VARCHAR(512) COMMENT 'go-plugin 二进制文件路径或内置包名',
    grpc_port_range VARCHAR(32) COMMENT '预留 gRPC 端口段，仅外置进程模式使用',
    status TINYINT DEFAULT 1 COMMENT '0 禁用 1 启用',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

#### User（平台用户）
```sql
CREATE TABLE sys_user (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    username VARCHAR(64) NOT NULL UNIQUE COMMENT '登录名',
    display_name VARCHAR(128) COMMENT '显示名',
    email VARCHAR(128),
    phone VARCHAR(32),
    password_hash VARCHAR(256) NOT NULL COMMENT 'bcrypt/argon2 哈希',
    auth_source VARCHAR(32) DEFAULT 'local' COMMENT 'local / oidc / ldap / 插件ID',
    status TINYINT DEFAULT 1 COMMENT '0 禁用 1 启用 2 锁定',
    last_login_at TIMESTAMP NULL,
    last_login_ip VARCHAR(64),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

#### Role（自定义角色）
```sql
CREATE TABLE sys_role (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    code VARCHAR(64) NOT NULL UNIQUE COMMENT '角色编码，如 platform-admin',
    name VARCHAR(128) NOT NULL COMMENT '显示名',
    description VARCHAR(512),
    builtin TINYINT DEFAULT 0 COMMENT '0 自定义 1 内置不可删',
    status TINYINT DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

#### Permission（权限点定义）
```sql
CREATE TABLE sys_permission (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    code VARCHAR(128) NOT NULL UNIQUE COMMENT '权限点，如 clusters:read, resources:edit, backups:restore',
    module VARCHAR(64) NOT NULL COMMENT '所属模块：cluster/resource/backup/version/plugin/system',
    action VARCHAR(64) NOT NULL COMMENT '动作：list/get/create/update/delete/restore/enable/disable',
    name VARCHAR(128) NOT NULL COMMENT '显示名',
    description VARCHAR(512),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### RolePermission（角色-权限点关联）
```sql
CREATE TABLE sys_role_permission (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    role_id BIGINT NOT NULL,
    permission_code VARCHAR(128) NOT NULL,
    UNIQUE KEY uk_role_perm (role_id, permission_code)
);
```

#### UserRole（用户-角色关联，含数据范围）
```sql
CREATE TABLE sys_user_role (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT NOT NULL,
    role_id BIGINT NOT NULL,
    scope_type VARCHAR(32) NOT NULL DEFAULT 'platform' COMMENT 'platform / cluster / namespace',
    scope_cluster_code VARCHAR(64) COMMENT 'scope_type=cluster/namespace 时有效',
    scope_namespace VARCHAR(64) COMMENT 'scope_type=namespace 时有效',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_user_role_scope (user_id, role_id, scope_type, scope_cluster_code, scope_namespace)
);
```

#### AuditLog（操作审计日志）
```sql
CREATE TABLE audit_log (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    trace_id VARCHAR(64) COMMENT '请求追踪ID',
    user_id BIGINT,
    username VARCHAR(64),
    client_ip VARCHAR(64),
    user_agent VARCHAR(512),
    module VARCHAR(64) NOT NULL COMMENT 'cluster/resource/version/backup/plugin/system/auth',
    action VARCHAR(64) NOT NULL COMMENT 'login/logout/create/update/delete/restore/rollback/enable/disable',
    target_type VARCHAR(64) COMMENT '操作对象类型，如 Deployment、cluster_code',
    target_id VARCHAR(256) COMMENT '操作对象ID/名称',
    cluster_code VARCHAR(64),
    namespace VARCHAR(64),
    status VARCHAR(16) DEFAULT 'success' COMMENT 'success / fail',
    error_msg VARCHAR(1024),
    request_method VARCHAR(16),
    request_uri VARCHAR(512),
    request_body JSON COMMENT '脱敏后的请求体，敏感字段需替换为 ***',
    response_code INT,
    cost_ms INT COMMENT '耗时(ms)',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_created_at (created_at),
    INDEX idx_user_module (username, module, created_at),
    INDEX idx_cluster_target (cluster_code, target_type, created_at)
) COMMENT = '操作审计日志，建议按月分区或归档';
```

### 5.2 版本链设计
- 每个对象由 `(cluster_code, namespace, api_version, kind, name)` 唯一标识。
- 每次保存时，先写入新的 `resource_snapshot`，再清理超出 5 条的旧版本。
- 回滚操作：复制目标版本的 `raw_yaml` 应用回集群，并作为新版本写入。

---

## 6. 核心流程

### 6.1 导入集群
1. 用户上传 kubeconfig 或输入证书文本。
2. 服务端解析出 `api-server`、`client-cert`、`client-key`、`token`。
3. 加密存储原始 kubeconfig（字段级加密）。
4. 使用凭证尝试连接集群，获取版本与节点数。
5. 成功后保存集群元数据，失败则提示错误原因。

### 6.2 编辑资源
1. 用户在界面选择集群、命名空间、对象类型、对象名称。
2. 后端通过 Dynamic Client 获取当前对象 YAML。
3. 用户在 YAML 编辑器中修改并提交。
4. 后端先保存当前 YAML 到 `resource_snapshot`（作为上一版本）。
5. 应用新 YAML 到集群。
6. 成功后写入新版本快照，清理旧版本。

### 6.3 版本回滚
1. 用户打开对象版本历史，选择目标版本。
2. 展示目标版本与当前版本的 diff。
3. 确认后，将目标版本 YAML 应用至集群（server-side apply 或 replace）。
4. 成功后写入一次新的 snapshot，source 标记为 `rollback`。

### 6.4 备份与恢复
1. 用户选择备份对象/命名空间，提交备份任务。
2. Backup Manager 读取资源 YAML，打包为 tar.gz / 单文件 YAML。
3. 写入配置的存储后端，记录元数据到 `backup_task`。
4. 恢复时读取备份文件，逐个应用 YAML 到目标集群。

---

## 7. 接口设计（关键 API）

### 7.1 集群管理
```
POST   /api/v1/clusters              # 导入集群
GET    /api/v1/clusters               # 集群列表
GET    /api/v1/clusters/{code}        # 集群详情
PUT    /api/v1/clusters/{code}        # 更新证书/信息
DELETE /api/v1/clusters/{code}        # 删除集群
POST   /api/v1/clusters/{code}/ping   # 连通性检测
```

### 7.2 资源对象
```
GET    /api/v1/clusters/{code}/resources/{apiVersion}/{kind}
GET    /api/v1/clusters/{code}/resources/{apiVersion}/{kind}/{namespace}/{name}
POST   /api/v1/clusters/{code}/resources/{apiVersion}/{kind}
PUT    /api/v1/clusters/{code}/resources/{apiVersion}/{kind}/{namespace}/{name}
DELETE /api/v1/clusters/{code}/resources/{apiVersion}/{kind}/{namespace}/{name}
```

### 7.3 版本历史
```
GET    /api/v1/clusters/{code}/resources/{apiVersion}/{kind}/{namespace}/{name}/versions
GET    /api/v1/clusters/{code}/resources/{apiVersion}/{kind}/{namespace}/{name}/versions/{seq}/diff
POST   /api/v1/clusters/{code}/resources/{apiVersion}/{kind}/{namespace}/{name}/versions/{seq}/rollback
```

### 7.4 备份管理
```
POST   /api/v1/clusters/{code}/backups
GET    /api/v1/backups
GET    /api/v1/backups/{id}
POST   /api/v1/backups/{id}/restore
```

### 7.5 插件中心
```
GET    /api/v1/plugins
POST   /api/v1/plugins/{pluginId}/enable
POST   /api/v1/plugins/{pluginId}/disable
GET    /api/v1/plugins/{pluginId}/capabilities
```

---

## 8. 插件式扩展设计

### 8.1 插件运行时架构（hashicorp/go-plugin + gRPC）

MVP 阶段插件以内置 Interface + 代码注册方式实现，但所有接口契约严格按 go-plugin(gRPC) 规范定义。后续升级为独立进程插件时只需替换 Plugin Manager 的加载策略，业务代码零改动。

```
┌─────────────────────────────────────────────────────────────────────┐
│ API Server / Task Worker (主进程)                                    │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │                    Plugin Manager                           │   │
│   │  ┌───────────────┐  ┌────────────────┐  ┌────────────────┐ │   │
│   │  │ 内置插件加载器 │  │ gRPC子进程管理器 │  │  能力注册中心  │ │   │
│   │  │ (MVP 代码注册) │  │ (后续升级启用)   │  │  CapRegistry  │ │   │
│   │  └───────┬───────┘  └───────┬────────┘  └───────┬────────┘ │   │
│   └──────────┼──────────────────┼───────────────────┼──────────┘   │
│              │                  │                   │               │
│   ┌──────────▼──────────────────▼───────────────────▼──────────┐   │
│   │                  统一 gRPC Client 代理层                    │   │
│   │   IResourceHandler │ IBackupStorage │ IAuthProvider        │   │
│   │   INotifier        │ IAuditExporter  │ IKMSProvider        │   │
│   └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
                           │ gRPC (子进程模式)
                           │
              ┌────────────┴────────────┐
              ▼                         ▼
    ┌──────────────────┐      ┌──────────────────┐
    │ plugin-ingress   │      │ plugin-s3        │
    │ (独立 Go 进程)    │      │ (独立 Go 进程)    │
    │ gRPC Server      │      │ gRPC Server      │
    └──────────────────┘      └──────────────────┘
```

**进程管理策略：**
- 主进程启动时根据 `plugin_registry` 扫描可用插件
- 内置插件：直接调用 Go 结构体方法，零序列化开销
- 外置插件：`exec.Command` 启动子进程，双向 gRPC 通信 + 健康心跳
- 子进程崩溃自动重启，最多 3 次后标记插件为禁用，不影响主流程

### 8.2 插件类型

| 类型 | Plugin Kind | 说明 |
|------|-------------|------|
| ResourcePlugin | resource | 扩展新的 K8S 对象类型，声明 GVK、列表字段、详情模板、自定义操作按钮。 |
| BackupStoragePlugin | backup_storage | 扩展备份存储目标，如 S3、MinIO、NFS、FTP、云盘。 |
| AuthPlugin | auth | 扩展认证方式，如 OIDC、LDAP、OAuth2、CAS、SAML。 |
| NotifierPlugin | notifier | 扩展通知通道，如邮件、钉钉、企业微信、Slack、飞书、Webhook。 |
| AuditExporterPlugin | audit | 自定义审计日志导出格式或转发目标（ES/Kafka/SIEM）。 |
| KMSPlugin | kms | 自定义密钥托管，对接 Vault、云厂商 KMS、HSM。 |

### 8.3 插件契约（ResourcePlugin 示例 manifest.json）
```json
{
  "pluginId": "custom-ingress-plugin",
  "name": "Ingress 管理扩展",
  "version": "1.0.0",
  "kind": "resource",
  "apiVersion": "plugin.k8s-platform.io/v1",
  "requires": {
    "platformVersion": ">=1.0.0",
    "dependencies": []
  },
  "capabilities": {
    "supportedKinds": ["Ingress"],
    "apiVersions": ["networking.k8s.io/v1"],
    "listColumns": [
      {"key": "name", "label": "名称", "sortable": true},
      {"key": "namespace", "label": "命名空间"},
      {"key": "rules", "label": "路由规则", "template": "custom"},
      {"key": "age", "label": "存活时间", "type": "age"}
    ],
    "actions": ["view", "edit", "delete", "custom:restart"],
    "icon": "NetworkOutlined",
    "detailTemplate": "plugin-templates/ingress-detail.tsx"
  },
  "entry": {
    "builtin": "internal/plugins/resource/ingress",
    "grpc": "bin/plugins/ingress-plugin"
  },
  "configSchema": {
    "type": "object",
    "properties": {}
  }
}
```

### 8.4 后端插件接口契约（Go Interface → gRPC proto）

所有插件接口必须满足：入参/出参可序列化（struct + proto3）、上下文传递、错误码标准化。

```go
// 8.4.1 资源类型插件：扩展新的 K8S GVK 支持
type IResourceHandler interface {
    // 返回插件支持的 GVK 列表
    SupportedGVKs(ctx context.Context) ([]GVKMeta, error)
    // 对象列表前的自定义过滤/转换（可选）
    BeforeList(ctx context.Context, req *ListRequest) (*ListRequest, error)
    // 列表返回后的字段 enrich（如自定义列计算）
    AfterList(ctx context.Context, items []*unstructured.Unstructured) ([]ResourceItem, error)
    // YAML 保存前的自定义校验（返回阻断错误则不保存）
    BeforeSave(ctx context.Context, obj *unstructured.Unstructured) error
    // YAML 保存后的后置动作（如触发通知、清理关联资源）
    AfterSave(ctx context.Context, obj *unstructured.Unstructured, oldObj *unstructured.Unstructured) error
    // 自定义操作按钮的执行入口
    CustomAction(ctx context.Context, action string, obj *unstructured.Unstructured) (*ActionResult, error)
}

// 8.4.2 备份存储插件：Local / S3 / NFS / FTP
type IBackupStorage interface {
    // 存储类型标识，对应 backup_task.storage_type
    StorageType(ctx context.Context) (string, error)
    // 上传备份文件（key 如：clusterA/2024-08-22/deploy-nginx.yaml）
    Upload(ctx context.Context, key string, reader io.Reader, opts *UploadOptions) (*UploadResult, error)
    // 下载备份文件
    Download(ctx context.Context, key string) (io.ReadCloser, *FileMeta, error)
    // 删除备份
    Delete(ctx context.Context, key string) error
    // 列出备份（按前缀/时间筛选）
    List(ctx context.Context, prefix string, opts *ListOptions) ([]*FileMeta, error)
    // 连通性检测（设置页点击测试）
    Ping(ctx context.Context) error
}

// 8.4.3 认证插件：OIDC / LDAP / OAuth2
type IAuthProvider interface {
    // 登录入口：返回跳转URL或直接验证凭证
    Authenticate(ctx context.Context, req *AuthRequest) (*AuthResult, error)
    // 根据 Token / Session 获取用户信息
    GetUserInfo(ctx context.Context, token string) (*UserInfo, error)
    // 回调处理（OAuth2 / OIDC）
    Callback(ctx context.Context, req *CallbackRequest) (*AuthResult, error)
    // 登出清理
    Logout(ctx context.Context, token string) error
}

// 8.4.4 通知插件：邮件 / 钉钉 / 企微 / Slack
type INotifier interface {
    // 通道类型
    ChannelType(ctx context.Context) (string, error)
    // 发送通知（模板 + 变量）
    Send(ctx context.Context, req *NotifyRequest) error
    // 批量发送
    BatchSend(ctx context.Context, reqs []*NotifyRequest) ([]*SendResult, error)
}

// 8.4.5 审计导出插件：ES / Kafka / SIEM
type IAuditExporter interface {
    // 导出批量审计日志
    Export(ctx context.Context, logs []*AuditLogEntry) error
    // 健康检测
    Ping(ctx context.Context) error
}

// 8.4.6 KMS 插件：本地 AES / Vault / 云厂商 KMS
type IKMSProvider interface {
    // 加密明文
    Encrypt(ctx context.Context, plaintext []byte, aad []byte) (ciphertext []byte, err error)
    // 解密密文
    Decrypt(ctx context.Context, ciphertext []byte, aad []byte) (plaintext []byte, err error)
    // 返回使用的 Key 版本（用于密钥轮换标识）
    KeyVersion(ctx context.Context) (string, error)
}
```

### 8.5 插件生命周期

```
REGISTER → LOAD → INIT(注入 Config + Host API) → ENABLE ↔ DISABLE → UNLOAD
```

| 阶段 | 触发时机 | 插件行为 |
|------|----------|----------|
| REGISTER | 主进程启动或插件安装 | 读取 manifest.json，写入 plugin_registry，校验依赖 |
| LOAD | 首次调用或启用时 | 内置：反射实例化；外置：fork 子进程 + gRPC 握手 |
| INIT | 加载完成后 | 插件 Init 方法，注入 PluginHostAPI（回调用，如写审计、发通知、查 RBAC） |
| ENABLE/DISABLE | 插件中心点击 | 标记状态，拦截调用链（禁用直接返回错误） |
| UNLOAD | 进程退出或插件卸载 | Graceful Shutdown：关闭 gRPC、释放文件句柄、清理临时资源 |

---

## 9. 安全设计

### 9.1 证书安全（AES-256-GCM + K8S Secret 挂载）

**加密方案：**
- 算法：AES-256-GCM（AEAD，带认证标签，防篡改）
- 密钥长度：32 字节（256 bit）
- Nonce：12 字节，加密时随机生成，**不得复用**，与密文拼接存储
- AAD（附加认证数据）：使用 `cluster_code + user_id(创建者)`，防止跨集群密文迁移
- 存储格式（Base64 编码后存 DB）：
  ```
  [1字节版本=0x01][12字节Nonce][密文][16字节GCM Tag]
  ```

**密钥托管：**
- 密钥通过 K8S Secret 以文件形式挂载到容器指定路径（如 `/etc/k8s-platform/encryption.key`）
- 启动时只读加载一次到内存变量，文件句柄立即关闭
- 支持热轮换：新增环境变量 `KEY_VERSION=v2`，解密时尝试当前密钥 + 历史密钥（上一版本），加密统一用新版本
- 密钥文件权限：`0400`，容器内用户非 root

**内存安全：**
- 解密后的 kubeconfig 仅保留在 Dynamic Client 的 `rest.Config` 结构体作用域内
- 操作完成后，返回连接池前调用显式 `zeroOutBuffer()` 清理敏感字段内存
- **禁止**以任何形式打印、日志、上报明文 kubeconfig（即使 debug 级别也不行）

**凭证过期提醒：**
- 导入集群时解析 client-cert / client-key / token 的失效时间
- Cluster Health Worker 每日巡检，30/7/1 天内过期分别在 UI 标黄/橙/红 + 发送 Notifier 通知

### 9.2 权限控制（RBAC 权限点 + 自定义角色 + 数据范围）

平台权限分为三层校验，**层层递进**，任何一层不通过即拒绝：

```
                请求进入
                   │
                   ▼
        ┌─────────────────────┐
        │ 1. 认证校验 (JWT)   │ → 未登录 → 401
        │   token 有效性校验  │
        └──────────┬──────────┘
                   ▼
        ┌─────────────────────┐
        │ 2. 功能权限点校验    │ → 无权限 → 403
        │ sys_permission.code │   判断用户是否绑定的角色包含目标权限点
        └──────────┬──────────┘
                   ▼
        ┌─────────────────────┐
        │ 3. 数据范围校验      │ → 越权 → 403
        │ sys_user_role.scope │   platform/cluster/namespace 三档
        └──────────┬──────────┘
                   ▼
              执行业务逻辑
```

**9.2.1 内置角色与权限点（初始化种子数据）**

| 内置角色 code | 说明 | 默认授予权限点举例 |
|---|---|---|
| `platform-admin` | 平台管理员（内置不可删） | `*:*` 所有模块所有动作 |
| `cluster-admin` | 集群管理员 | clusters:*, resources:*, version:*, backup:* |
| `namespace-developer` | 命名空间开发者 | resources:list/get/create/update, version:diff, backup:restore-self |
| `platform-viewer` | 平台只读用户 | 所有模块的 `:list / :get` |

**预置权限点清单（MVP 阶段）：**

| module | action | 权限点 code | 说明 |
|---|---|---|---|
| auth | login | `auth:login` | 登录登出 |
| cluster | list | `cluster:list` | 集群列表 |
| cluster | create | `cluster:create` | 导入集群 |
| cluster | update | `cluster:update` | 更新证书/信息 |
| cluster | delete | `cluster:delete` | 删除集群 |
| cluster | ping | `cluster:ping` | 连通性检测 |
| resource | list | `resource:list` | 资源列表 |
| resource | get | `resource:get` | 资源详情 |
| resource | create | `resource:create` | 新建资源 |
| resource | update | `resource:update` | YAML 编辑保存 |
| resource | delete | `resource:delete` | 删除资源 |
| version | list | `version:list` | 版本历史 |
| version | diff | `version:diff` | 版本对比 |
| version | rollback | `version:rollback` | 版本回滚 |
| backup | list | `backup:list` | 备份列表 |
| backup | create | `backup:create` | 新建备份 |
| backup | restore | `backup:restore` | 从备份恢复 |
| backup | delete | `backup:delete` | 删除备份 |
| plugin | list | `plugin:list` | 插件中心 |
| plugin | enable | `plugin:enable` | 启用插件 |
| system | config | `system:config` | 系统设置 |
| audit | list | `audit:list` | 审计日志查询 |

**9.2.2 数据范围（scope）**

`sys_user_role` 中定义的 `scope_type` 决定用户可见/操作的对象范围：

| scope_type | 生效条件 | 说明 |
|---|---|---|
| `platform` | 全平台 | 能看到所有集群、所有命名空间 |
| `cluster` | scope_cluster_code 非空 | 仅能操作指定集群，跨集群 API 返回 403 |
| `namespace` | scope_cluster_code + scope_namespace 非空 | 仅能操作该集群该命名空间，且无法操作集群级资源（PV、StorageClass、ClusterRole） |

> **与 K8S 原生 RBAC 的关系**：平台层校验完权限后，实际操作使用导入的 kubeconfig 凭证调用 K8S API。若 kubeconfig 本身权限不足，K8S API Server 会再次拒绝，平台负责将 K8S 返回的 `Forbidden` 错误翻译为用户可读提示。平台层**不替代** K8S RBAC，而是作为第一道闸门 + 审计入口。

**9.2.3 接口级权限校验实现**

所有需要鉴权的路由统一加两层中间件（Gin Handler Chain）：
```
[AuthMiddleware(JWT解析)] → [RBAC Middleware(权限点+Scope)] → Handler
```
- RBAC Middleware 从 JWT 提取 `user_id`，通过 Redis 缓存的用户角色权限树做匹配（减少 DB 查）
- 权限变更（角色编辑、用户绑定）后主动失效 Redis key

### 9.3 操作审计

**写入链路：**
```
Handler 返回前 → Audit Middleware 收集上下文
    → 组装 AuditLogEntry（body 脱敏）
    → 异步写 PostgreSQL audit_log 表（不阻塞主请求，失败降级写 stdout）
    → [可选] 通过 IAuditExporterPlugin 转发至 ES/Kafka/SIEM
```

**脱敏规则（写入 request_body 前执行）：**
- 匹配字段名：`password / token / secret / key / credential / bearer` → 值替换为 `***`
- 匹配正则：`-----BEGIN.*PRIVATE KEY-----.*-----END.*PRIVATE KEY-----` → 整块替换
- JSON 多层递归扫描，不遗漏嵌套字段

**查询入口：**
- 平台管理员可见全量审计日志
- 普通用户仅可见自身操作记录
- 支持按时间范围 / 用户 / 模块 / 动作 / 集群 / 状态 多条件筛选
- 大表建议按月分区或定期归档到冷存储

### 9.4 网络安全

- 前端 → API Server：强制 HTTPS（TLS 1.2+），HSTS 头
- 内部 API Server ↔ Task Worker：同一 K8S 集群内通过 ClusterIP 通信；生产部署启用 NetworkPolicy 仅放通需要的端口
- API Server → K8S API Server：优先走 kubeconfig 自带的 TLS 链路；API Server 不可直连时支持配置 HTTPS_PROXY / SOCKS5 代理 / kubectl proxy sidecar
- CORS：仅白名单域名允许跨域；携带 Cookie 的请求走 SameSite

---

## 10. 前端信息架构

### 10.1 页面/模块清单
| 页面 | 功能 |
|------|------|
| 集群总览 | 已导入集群卡片、状态、快速入口。 |
| 集群导入 | 上传 kubeconfig / 粘贴证书、连通性检测。 |
| 资源对象列表 | 按对象类型筛选、命名空间筛选、搜索、分页。 |
| 资源详情 & YAML 编辑 | 展示 YAML/表单、编辑、保存、删除。 |
| 版本历史 | 时间轴展示最近 5 个版本、对比、回滚。 |
| 备份列表 | 备份任务状态、大小、操作人、恢复入口。 |
| 新建备份 | 选择对象/命名空间、存储后端、立即执行。 |
| 插件中心 | 已安装插件、能力、启用/禁用。 |
| 系统设置 | 通知、审计、存储配置（管理员）。 |

### 10.2 导航结构
```
├── 概览
├── 集群管理
│   ├── 集群列表
│   └── 导入集群
├── 资源管理
│   ├── Workload
│   │   ├── Deployments
│   │   ├── StatefulSets
│   │   ├── DaemonSets
│   │   ├── Jobs
│   │   └── CronJobs
│   ├── 配置 (ConfigMaps / Secrets)
│   └── 存储 (PV / PVC / StorageClass)
├── 备份管理
│   ├── 备份列表
│   └── 新建备份
├── 插件中心
└── 系统设置
```

---

## 11. 技术栈（已选型确认）

### 11.1 后端技术栈

| 层次 | 选型 | 理由 |
|------|------|------|
| 语言 | **Go 1.25.5** | 云原生生态首选；client-go 官方支持最好；静态编译单二进制无依赖；并发性能高；内存占用低。 |
| Web 框架 | **Gin** | Go 生态最成熟、社区最活跃的 HTTP 框架；中间件生态完善；性能优于 Beego/Iris。 |
| K8S 客户端 | **client-go** | 官方唯一推荐；Dynamic Client + Unstructured 支持任意 GVK；自带 SharedInformer 缓存工厂。 |
| K8S 连接管理 | **Dynamic Client 连接池 + SharedInformer(按需)** | 按 cluster_code 缓存 `*rest.Config` + `dynamic.Interface`；Informer 仅在用户首次访问对应 GVK 时 Lazy Start。 |
| 数据库 | **PostgreSQL 15+** | JSONB 字段完美承载插件 capabilities、备份元数据；支持复杂查询 + 窗口函数（版本链清理）；事务 ACID 完整。 |
| ORM / 数据访问 | **GORM / pgx (sqlx)** | 复杂查询推荐 pgx 原生 SQL + 扫描；简单 CRUD 用 GORM。两者可共存。 |
| 缓存 & 队列 | **Redis 7+** | 双用途：1) RBAC 权限树、Informer 状态、限流计数器缓存；2) Redis Stream 做备份任务队列 + 死信。 |
| 任务队列 | **Redis Stream** | 无额外中间件依赖；Consumer Group 支持水平扩缩容的 Worker；ACK/Pending 机制保证任务可靠；XAUTOCLAIM 做死信转移。 |
| 插件框架 | **hashicorp/go-plugin (gRPC 模式)** | MVP 阶段用内置代码注册（零序列化开销），但接口契约 100% 遵循 gRPC proto；后续升级为子进程模式时业务零改动。 |
| 序列化 | **protobuf + encoding/json** | 插件接口用 proto3（跨语言、向后兼容）；HTTP API 用 JSON。 |
| 加密 | **AES-256-GCM** | AEAD 认证加密防篡改；Go crypto/aes 原生支持；Nonce 随机生成不复用。 |
| JWT | **golang-jwt/jwt/v5** | 社区主流实现；支持 RS256/HS256；v5 支持最新规范。 |
| 配置管理 | **Viper** | 支持 YAML/JSON/TOML；K8S ConfigMap 热加载；优先级：命令行 > 环境变量 > 配置文件。 |
| 日志 | **Zap + Lumberjack** | 结构化 JSON 日志（stdout 输出给 Loki/ELK 采集）；error 级别单独落文件；支持 trace_id 透传。 |
| 错误码 | **自定义错误包 + HTTP 映射** | 统一 Code/Message/Details 结构；400 客户端参数 / 401 未认证 / 403 无权限 / 404 不存在 / 409 冲突 / 500 系统。 |
| 限流熔断 | **golang.org/x/time/rate + go-breaker** | Gin 中间件按用户/IP 令牌桶限流；对 K8S API 调用端加熔断器，故障时快速降级。 |
| 迁移工具 | **golang-migrate / goose** | SQL 原生迁移脚本；支持 Up/Down；可嵌入二进制。 |

### 11.2 前端技术栈

| 层次 | 选型 | 理由 |
|------|------|------|
| 框架 | **React 18+ (TypeScript)** | Ant Design Pro 原生框架；社区组件最丰富；Monaco Editor 集成最成熟。 |
| 脚手架 | **Ant Design Pro (v6+)** | 内置 ProLayout / ProTable / 权限路由 / Mock / i18n；开箱即用的管理后台模板。 |
| 状态管理 | **Redux Toolkit + RTK Query** | AntD Pro 默认方案；RTK Query 自动做请求缓存、去重、轮询，替代手写 service 层。 |
| YAML 编辑器 | **Monaco Editor + @monaco-editor/react + yaml-language-server** | 语法高亮、YAML Schema 校验、自动补全、折叠；与 VS Code 同款体验。 |
| Diff 展示 | **Monaco Diff Editor**（版本对比） + **diff2html**（行内 diff 摘要） | 大 YAML 场景下 Monaco 性能明显优于纯 diff2html；左右对照体验好。 |
| HTTP 客户端 | **Axios (封装 request.ts + 拦截器)** | JWT 自动注入、401 自动刷新、统一错误处理、trace_id 透传。 |
| WebSocket | **原生 WebSocket + reconnecting-websocket** | 自动重连 + 心跳；承载 Pod 日志流 / 任务进度 / 集群事件三条 Channel。 |
| 图表 | **AntV G2Plot / @ant-design/charts** | 与 AntD 设计语言统一；集群节点/资源状态看板够用。 |
| 表单 | **ProForm + react-hook-form (复杂场景)** | ProForm 快速 CRUD；复杂动态表单用 react-hook-form + zod 校验。 |
| 构建工具 | **Vite**（或 AntD Pro 默认 Umi） | Vite 开发启动快；Umi 与 AntD Pro 集成更紧。优先 Vite。 |
| 代码规范 | **ESLint + Prettier + Husky + lint-staged** | 提交前自动格式化 + lint；TypeScript strict 模式开启。 |

### 11.3 部署与运维

| 层次 | 选型 | 理由 |
|------|------|------|
| 容器化 | **Docker + multi-stage build** | 构建阶段编译 Go + 前端产物；运行阶段基于 distroless/alpine，镜像控制在 200MB 内。 |
| 部署 | **Kubernetes (Helm Chart)** | 平台自身也运行在 K8S 上；Helm Chart 管理版本、可配置 values.yaml。 |
| 部署形态 | **API Server Deployment(≥2) + Task Worker Deployment(≥1)** | 两组件独立扩缩容；API Server HPA 按 CPU；Worker HPA 按 Redis Stream pending 长度。 |
| 镜像仓库 | **Harbor / 云厂商容器镜像服务** | 私有化场景推荐 Harbor；公有云用对应厂商 ACR/ACK/TCR。 |
| CI/CD | **GitHub Actions / GitLab CI** | 三阶段：lint-test → build-push-image → helm deploy；PR 触发预览环境。 |
| 可观测 | **Prometheus + Grafana + Loki** | /metrics 暴露 Go runtime、API 延迟、队列堆积、Informer 同步延迟；Loki 采集结构化日志。 |
| 前端静态资源 | **Nginx Ingress + CDN** | 构建产物打包进 API Server 二进制（embed.FS）或独立上传 OSS + CDN。 |
| Secret 注入 | **K8S Secret → Volume 挂载** | 数据库密码、Redis 密码、AES 密钥均通过 K8S Secret 挂载文件，**严禁**写在镜像或 ConfigMap 明文。 |

### 11.4 架构决策总表（供快速查阅）

| 决策项 | 结果 |
|---|---|
| 后端语言/框架 | Go 1.25.5 / Gin |
| 数据库 | PostgreSQL 15+ |
| K8S 连接策略 | Dynamic Client 连接池 + SharedInformer(按需Lazy Load) |
| 缓存 & 任务队列 | Redis 7+ / Redis Stream |
| 插件架构 | go-plugin(gRPC) 契约 + MVP 内置代码注册 |
| 备份存储 | 本地磁盘（MVP） + S3 兼容 + NFS（3 个 IBackupStorage 实现） |
| 认证 | 本地账号密码 + JWT + IAuthProvider 插件点预留 |
| 密钥托管 | AES-256-GCM + K8S Secret 挂载密钥文件 |
| RBAC 模型 | 权限点 + 自定义角色 + scope(platform/cluster/namespace) 三档 |
| 审计日志 | PostgreSQL audit_log 表 + 异步写入 + 脱敏 |
| 部署形态 | API Server + Task Worker 双 Deployment 分离 |
| 前端框架 | React 18 + Ant Design Pro v6 |
| 前端状态 | Redux Toolkit + RTK Query |
| YAML 编辑器 | Monaco Editor + yaml-language-server |
| WebSocket 场景 | Pod 日志流 · 任务进度推送 · 集群事件通知 |
| Informer 保护策略 | 按需懒加载 + List 限流 + API Server 熔断（见第 14 节） |

---

## 12. 第一阶段实现范围（MVP）

为快速验证价值，建议第一期聚焦：
1. 集群导入与连接检测。
2. 工作负载（Deployment / StatefulSet / DaemonSet / Job / CronJob）的列表、详情、YAML 编辑、删除。
3. ConfigMap / PV / PVC / StorageClass 的列表、详情、YAML 编辑。
4. 资源修改时自动生成版本快照，保留最近 5 个版本，支持版本对比与回滚。
5. 单对象 YAML 备份与恢复（本地存储）。
6. 插件中心框架与 ResourcePlugin 扩展点（至少预留接口，可内置实现）。

---

## 13. 风险与应对

| 风险 | 应对 |
|------|------|
| K8S API 权限过大 | 明确告知用户导入的凭证权限即平台操作权限；后续支持只读/读写模式。 |
| 证书泄露 | 加密 + KMS；定期轮换；操作审计。 |
| 大集群资源列表卡顿 | Informer 本地缓存 + 分页 + 搜索后端化。 |
| 版本回滚失败 | 使用 server-side apply；失败时给出明确错误与原始 YAML 下载。 |
| 插件接口不稳定 | 版本化插件契约；内置插件优先；向后兼容。 |
| Informer 全量同步压垮 API Server | 见第 14 节专项机制：按需懒加载 + List QPS 限流 + 熔断器 + 优雅降级。 |

---

## 14. Informer 缓存保护机制专项（API Server 保护）

> 核心原则：**平台不能因为自己的缓存同步行为，把用户业务集群的 API Server 拉挂。** 所有 Informer 启动、List、Watch 环节均必须带保护措施。

### 14.1 触发场景与风险分析

| 场景 | 风险级别 | 说明 |
|---|---|---|
| 导入大集群（5k+ Pod / 1k+ Deployment）后立即启动全量 Informer | ⚠️ 高危 | List 阶段一次拉几万条对象，API Server etcd 读压力暴增；多个 GVK 同时启动更易触发 API Server OOM 或限流 |
| 平台重启后恢复所有 Informer | ⚠️ 高危 | 同上，冷启动瞬时高并发 List |
| Informer Watch 断线重连频繁 | 🟡 中危 | 每次重连 = 一次 Relist，网络抖动下易放大成连续 List 风暴 |
| 用户反复切换不同 GVK 列表页 | 🟡 中危 | 懒加载模式下，每切一种 GVK 触发一次 List，短时间多次切换=多次 List |
| 大集群单 Namespace 筛选 | 🟢 低危 | 如果 Informer 全局缓存了所有 NS，List 实际走本地 indexer，不打 API Server |

### 14.2 六层防护架构

```
┌───────────────────────────────────────────────────────────────────┐
│ Layer 1  按需懒启动（Lazy Start）                                   │
│   → 用户首次访问某 GVK 列表时才启动该 Informer；导入集群不预热        │
│   → 空闲 TTL（默认 30 分钟）无访问自动 Stop，释放内存 & Watch 连接   │
└──────────────────────────────────┬────────────────────────────────┘
                                   ▼
┌───────────────────────────────────────────────────────────────────┐
│ Layer 2  全局启动令牌桶（Global Startup Semaphore）                 │
│   → 同一 Worker 内同一时刻最多 N 个 Informer 同时处于 List 阶段      │
│   → 默认 N=2；超阈值排队，避免瞬时集中 List                          │
└──────────────────────────────────┬────────────────────────────────┘
                                   ▼
┌───────────────────────────────────────────────────────────────────┐
│ Layer 3  List 阶段 QPS/Burst 限流 + 分页 chunk                      │
│   → List 包装带 context.WithTimeout（默认 60s）                     │
│   → client-go List 配置 Limit=500 + Continue 分页拉；页间 sleep 1s  │
│   → 单集群所有 Informer 共享 1 个 rate.Limiter(QPS=5, Burst=20)    │
└──────────────────────────────────┬────────────────────────────────┘
                                   ▼
┌───────────────────────────────────────────────────────────────────┐
│ Layer 4  API Server 调用端熔断器（Circuit Breaker）                 │
│   → 每 cluster_code 独立熔断器窗口                                  │
│   → 阈值：连续 5 次 5xx / 超时 → Open（5 分钟）→ Half-Open 探测      │
│   → Open 状态：所有非强制操作（如 List）直接返回「集群繁忙，稍后重试」 │
└──────────────────────────────────┬────────────────────────────────┘
                                   ▼
┌───────────────────────────────────────────────────────────────────┐
│ Layer 5  Watch 断线退避重连（Exponential Backoff）                  │
│   → 第 1 次断线：wait 1s；2 次：2s；3 次：4s；… 上限 60s             │
│   → 断线 10+ 次后自动 Stop Informer + UI 标红，通知管理员排查        │
│   → Relist 策略：使用 ResourceVersion 增量，禁止无条件全量 Relist    │
└──────────────────────────────────┬────────────────────────────────┘
                                   ▼
┌───────────────────────────────────────────────────────────────────┐
│ Layer 6  优雅降级（Fallback to Direct API）                         │
│   → Informer 处于 List / CacheSync 期间，列表请求直接打 API Server   │
│     但加 Limit=100 + timeout=15s，且走同 Layer3 限流                │
│   → Informer 缓存就绪后切回本地缓存；用户无感切换                     │
│   → 熔断器 Open 时同样走降级直连 + 更大的 Limit 限制                  │
└───────────────────────────────────────────────────────────────────┘
```

### 14.3 Informer 状态机与 Redis 共享

多 Worker 部署下，通过 Redis 维护 Informer 启动状态，避免重复启动：

```
[IDLE] ──首次访问──► [STARTING] ──List分页中──► [CACHING] ──Sync完成──► [READY]
                                    │                          │
                                    │超时/失败                  │空闲>30min无访问
                                    ▼                          ▼
                                [STOPPED] ◄───────────────────
                                    ▲
                                    │手动/定时器重试
```

- Redis Hash Key：`informer:state:{cluster_code}:{gvk}`
- 字段：`status, started_at, synced_at, last_list_count, last_err, worker_id`
- Worker 启动时用 `SETNX` 抢锁，避免多 Worker 对同 (cluster, gvk) 重复 List
- List 进度（分页情况）实时写 Redis，前端「集群同步状态」页展示给用户

### 14.4 配置参数（可全局 / 可单集群覆盖）

| 参数 | 默认值 | 说明 |
|---|---|---|
| `informer.lazyStart` | true | 是否启用懒启动（生产强制 true） |
| `informer.idleTTL` | 30m | 空闲多久自动 Stop（0=不自动停） |
| `informer.startupConcurrency` | 2 | 同时处于 List 阶段的 Informer 数量 |
| `informer.listQPS` | 5 | 单集群 K8S API List 阶段 QPS（client-go rate limiter） |
| `informer.listBurst` | 20 | 突发峰值 |
| `informer.listPageSize` | 500 | List 分页每页条数 |
| `informer.listPageInterval` | 1s | 分页之间 sleep |
| `informer.listTimeout` | 60s | 单 List 请求超时 |
| `informer.watchBackoffMin` | 1s | Watch 断线最小退避 |
| `informer.watchBackoffMax` | 60s | Watch 断线最大退避 |
| `informer.watchMaxRetries` | 10 | 断线最大重试次数（超过停 Informer） |
| `breaker.openThreshold` | 5 | 连续失败多少次开熔断 |
| `breaker.openDuration` | 5m | 熔断持续时间后进入半开 |
| `fallback.directAPILimit` | 100 | 降级直连 API Server 时的 List Limit |

### 14.5 可观测性（暴露给 Prometheus）

| Metric | 类型 | 标签 | 说明 |
|---|---|---|---|
| `k8s_platform_informer_start_total` | Counter | cluster_code, gvk, result | Informer 启动次数（success/timeout/fail） |
| `k8s_platform_informer_stop_total` | Counter | cluster_code, gvk, reason | Informer 停止次数（idle/error/manual） |
| `k8s_platform_informer_list_duration_seconds` | Histogram | cluster_code, gvk, step=page/all | List 耗时分布（分页 + 整体） |
| `k8s_platform_informer_list_items_total` | Counter | cluster_code, gvk | List 拉取的对象总数（观察一次同步数据量） |
| `k8s_platform_informer_watch_disconnect_total` | Counter | cluster_code, gvk | Watch 断线次数（连续高则网络不稳） |
| `k8s_platform_apiserver_breaker_state` | Gauge | cluster_code | 0=Closed 1=HalfOpen 2=Open |
| `k8s_platform_fallback_requests_total` | Counter | cluster_code, gvk, reason | 降级直连 API Server 次数（informner_not_ready / breaker_open） |

> **关键告警规则**：
> - `breaker_state == 2` 持续 3 分钟 → 严重告警（API Server 出问题或限流了）
> - `watch_disconnect_total` 5 分钟内 > 20 次 → 警告（网络或 API Server 抖动）
> - `list_duration_seconds{step="all"}` p95 > 120s → 警告（列表同步太慢，需调小 pageSize 或降 QPS）

### 14.6 MVP 第一阶段最低实现

即使 MVP 时间紧，**至少必须实现以下 4 条**（不做即有拉挂 API Server 风险）：
1. ✅ **Lazy Start**：导入集群绝不自动启动 Informer；仅首访问触发
2. ✅ **List 分页 + 限流**：client-go 配置 `Limit + QPS/Burst + rate.Limiter`
3. ✅ **单集群并发控制**：用 channel/buffered semaphore 保证 List 阶段同时不超过 2 个 GVK
4. ✅ **List 超时 + 上下文取消**：用户切走页面/关闭浏览器时，`cancel()` 中止正在进行的 List

其余 Layers（熔断器、Watch Backoff、Redis 共享状态）可进入 v1.1 迭代补齐。

---

## 15. 技术架构决策记录（Appendix ADR）

> 本节记录本次澄清会议确认的架构决策，方便后续回顾「为什么选 A 不选 B」。

| ADR-编号 | 决策日期 | 决策项 | 决策结果 | 主要放弃方案 | 理由 / 备注 |
|---|---|---|---|---|---|
| ADR-001 | 2026-08-22 | 后端语言/框架 | Go 1.25.5 / Gin | Java/Spring、Node/NestJS | client-go 官方绑定最佳；单二进制部署 |
| ADR-002 | 2026-08-22 | 元数据库 | PostgreSQL 15+ | MySQL 8、MongoDB | JSONB + 窗口函数适合版本链和插件元数据 |
| ADR-003 | 2026-08-22 | K8S 连接 & 缓存 | 连接池 + Informer(Lazy) | 纯按需 Client、纯 Informer 全量 | 折中兼顾 API Server 压力与列表性能 |
| ADR-004 | 2026-08-22 | 用户认证 | 本地账号 + JWT + AuthPlugin 预留 | 纯 OIDC、LDAP 直连 | MVP 速度 + 后续 SSO 扩展能力 |
| ADR-005 | 2026-08-22 | 密钥托管 | AES-256-GCM + K8S Secret 挂载 | Vault、云厂商 KMS | 无额外组件依赖；KMS 接口已插件化可替换 |
| ADR-006 | 2026-08-22 | 备份存储实现 | Local + S3 + NFS 3 套 IBackupStorage | 仅 MVP Local | 生产环境诉求明确，直接 3 套实现 |
| ADR-007 | 2026-08-22 | 插件架构 | go-plugin(gRPC) 契约 + MVP 内置注册 | Go plugin(.so)、WASM | .so 跨版本兼容差；内置 MVP + 契约化 gRPC 渐进升级 |
| ADR-008 | 2026-08-22 | 任务队列 | Redis Stream | NSQ、RabbitMQ、Asynq | 不引入新中间件；Consumer Group 扩缩容能力够用 |
| ADR-009 | 2026-08-22 | 部署形态 | API Server + Task Worker 分离 | 单二进制 All-in-One、全微服务 | Worker 与 API 扩缩容维度不同；分离不过度复杂 |
| ADR-010 | 2026-08-22 | 前端框架 | React 18 + Ant Design Pro + Redux Toolkit | Vue3 + Element、Next.js | Monaco Editor/ProTable 生态最成熟 |
| ADR-011 | 2026-08-22 | 平台 RBAC 粒度 | 权限点 + 自定义角色 + 三档 scope | 固定 4 角色、两角色 MVP | 用户明确要求灵活自定义角色 |
| ADR-012 | 2026-08-22 | 审计日志存储 | PostgreSQL audit_log 表 + 异步写 | 仅 stdout 结构化日志 | 用户要求后台查询 UI；stdout 也同步输出 |
| ADR-013 | 2026-08-22 | WebSocket 场景 | Pod 日志 + 任务进度 + 集群事件 3 通道 | MVP 全轮询 | Pod 日志实时流是刚需 |
