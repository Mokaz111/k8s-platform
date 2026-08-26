# K8S Platform 架构文档

> 本文件深入描述 K8S Platform 多集群控制台的系统架构、模块划分、数据流、
> 部署拓扑与扩展点。面向架构师、贡献者、高级用户。
> 若你只关心如何使用，请直接回到 [README.md](../README.md)。

---

## 0. 架构目标与约束

### 0.1 设计目标
1. **多集群**：N 个 K8S 集群统一纳管，互不影响（故障隔离）
2. **可扩展**：插件化架构，新 GVK / 新存储后端 / 新认证方式零侵入扩展
3. **企业级**：RBAC 最小权限 + 全量审计 + 密钥加密 + 细粒度配额
4. **实时性**：备份进度、Pod 日志、集群事件通过 WebSocket 毫秒级推送
5. **易运维**：API Server 与 Worker 解耦，可独立扩缩容；Prometheus 友好

### 0.2 硬性约束
| 维度 | 约束 |
|------|------|
| 后端语言 | Go 1.25.5（CGO_ENABLED=0，纯静态链接） |
| 前端框架 | React 18 + TypeScript + Ant Design Pro v6 |
| 主数据库 | PostgreSQL 13+（存储元数据、用户、审计日志） |
| 缓存/队列/事件总线 | Redis 6.0+（Stream + PubSub + KV） |
| K8S 客户端 | client-go（DynamicClient + Informer） |
| Helm 集成 | k8s secrets 查询 + Helm CLI（避免 Go SDK 依赖） |
| 备份存储 | Local / S3 / NFS（插件化） |

---

## 1. 分层架构

```
┌──────────────────────────────────────────────────────────────────────┐
│  Layer 1: Presentation / Client                                      │
│  ┌──────────────────────┐   ┌──────────────────────────────────────┐ │
│  │ React SPA (AntD Pro) │   │   kubectl / curl / 第三方客户端      │ │
│  │ Redux / RTK Query    │   │                                      │ │
│  │ ReconnectingWS ×3    │   │   同样访问 /api/v1  RESTful 接口     │ │
│  └──────────┬───────────┘   └──────────────────┬───────────────────┘ │
└─────────────┼──────────────────────────────────┼─────────────────────┘
              │  HTTP + WebSocket (TLS)          │
┌─────────────▼──────────────────────────────────▼─────────────────────┐
│  Layer 2: API Server (Gin, stateless)                                │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ Middleware Stack                                                 │  │
│  │  CORS → Request-ID → Log → RateLimit → Audit → Auth(JWT+RBAC)  │  │
│  └────────────────────────────┬───────────────────────────────────┘  │
│                               ▼                                      │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ REST Routes (按领域模块聚合)                                     │  │
│  │  /auth/*    /clusters/*   /resources/*    /versions/*          │  │
│  │  /backups/* /helm/*       /rbac/users,roles  /audit/*          │  │
│  └────────────────────────────┬───────────────────────────────────┘  │
│                               ▼                                      │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │  WebSocket Hub (Gorilla WebSocket)                              │  │
│  │   channels: task_progress(200) / cluster_event(200) /          │  │
│  │             pod_logs(1000/pod)   [环形缓冲区 ring buffer]      │  │
│  └────────────────────────────┬───────────────────────────────────┘  │
│                               │                                      │
└───────────────────────────────┼──────────────────────────────────────┘
                                │ Redis PubSub (跨 API Server 广播)
┌───────────────────────────────┼──────────────────────────────────────┐
│  Layer 3: Business Modules  (接口化，依赖注入)                        │
│  ┌───────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌───────────┐ │
│  │ ClusterMgr│ │Resource  │ │VersionMgr│ │BackupMgr │ │  HelmMgr  │ │
│  │ (KubeConf │ │  Mgr     │ │ (Snapshots│ │ (Multi-  │ │ (secrets+ │ │
│  │  +AES-GCM │ │ (DynCli+ │ │  +Diff+  │ │  Storage │ │   CLI)    │ │
│  │  +Pool)   │ │Informer) │ │ Rollback)│ │  Plugins)│ │           │ │
│  └─────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ └─────┬─────┘ │
│        │            │            │             │             │        │
│        │     ┌──────▼────────────▼─────────────▼─────────────▼────┐   │
│        │     │          Plugin Manager (注册表 + 生命周期)        │   │
│        │     │   Storage / Auth / KMS / Notify / Audit / Resource│   │
│        │     └───────────────────────────────────────────────────┘   │
│        │                                                             │
│        └────────────────────────┬────────────────────────────────────┘
│                                 │
└─────────────────────────────────┼────────────────────────────────────┘
                                  │ PostgreSQL / Redis
┌─────────────────────────────────┼────────────────────────────────────┐
│  Layer 4: Task Worker (独立进程，水平扩展)                             │
│  ┌─────────────────┐ ┌──────────────────┐ ┌────────────────────────┐ │
│  │  Backup Worker  │ │ Informer Sync    │ │ Cluster Health Worker  │ │
│  │ (Redis Stream   │ │ (Lazy Start +    │ │ (Cron Ping + 失败     │ │
│  │  Consumer)      │ │  List QPS 限流)  │ │   计数 + 告警)         │ │
│  └─────────────────┘ └──────────────────┘ └────────────────────────┘ │
│         │                                                               │
└─────────┼──────────────────────────────────────────────────────────────┘
          │
          ▼  client-go (DynamicClient / REST Config)
┌───────────────────────────────────────────────────────────────────────┐
│  Layer 5: Target K8S Clusters (1..N, 跨厂商/私有云)                    │
│   Vendor-Managed  | 自建 | 边缘集群 | 多区域                           │
└───────────────────────────────────────────────────────────────────────┘
```

### 1.1 进程模型（为什么拆两个进程？）

| 进程 | 角色 | 端口 | 生命周期 | 扩缩容模式 |
|------|------|------|----------|-----------|
| **API Server** | 同步 API（HTTP/WS），无状态 | 8080 | HPA 按 CPU/Request QPS 水平扩展 | 无状态，N 副本 |
| **Task Worker** | 异步任务消费、Informer、健康检查 | 8081 | HPA 按 Redis 队列深度扩展 | 有状态，但可多副本竞争消费 |

**解耦原因**：
- API Server 要**低延迟**（p95 < 50ms 除长连接），不得被备份/Informer List 阻塞
- Informer Watch 对每个集群 1 条长连接，多副本时需领导者选举（Leader Election，
  目前用简单的 Redis 分布式锁，后续换 Redis Client `SET NX EX`）；API 进程不承担此职责
- 备份任务 CPU/IO 密集，高峰期独立扩容不会拖垮 API

---

## 2. 模块详解

### 2.1 Cluster Manager（集群管理）
**位置**：`backend/internal/cluster/manager.go`

职责：
- kubeconfig **解析**（从文件 / 文本）、**加密**（AES-GCM 256）、**持久化**（PostgreSQL）
- 为每个集群维护一个 `*rest.Config` + `dynamic.Interface` 缓存（连接池）
- Ping 连通性检测（版本、节点数、耗时）
- 对外提供：`ImportCluster / List / Get / Update / Delete / Ping / GetRestConfig / GetKubeConfig`

**加密模型**：
```
kubeconfig (明文)
   │
   ▼
crypto.AESGCM.Encrypt(kms_key, kubeconfig_bytes)
   │
   ├── ciphertext = AES-256-GCM(key_version + nonce + data + tag)
   │
   ▼
PostgreSQL: clusters.encrypted_kubeconfig (BLOB)
```

### 2.2 Resource Manager（资源对象管理）
**位置**：`backend/internal/resource/manager.go` + `informer_mgr.go`

职责：
- **同步路径**（直连 API Server，低延迟）：`Get / Create / Update / Delete`
- **列表路径**（Informer 缓存，降低 APIServer 压力）：`List / Watch`
- Informer Manager 控制参数（设计文档 9.2 节）：

| 参数 | 默认值 | 含义 |
|------|--------|------|
| `lazy_start` | true | 首次访问才启动 Informer |
| `idle_ttl` | 30m | 空闲自动停止释放 Watch |
| `startup_concurrency` | 2 | 同集群同阶段 List GVK 并发数 |
| `list_qps / list_burst` | 5 / 20 | 限流 |
| `list_page_size` | 500 | 分页大小 |
| `watch_backoff` | 1s → 60s | Watch 断连指数退避 |

### 2.3 Version Manager（版本历史）
**位置**：`backend/internal/version/manager.go`

- 每次 `resource:update` 前，将原 YAML 写入 `resource_version_snapshots` 表
- 每个对象保留最近 N=5 个快照（可配置），旧版本 `DELETE`
- 提供 `Diff(a,b)` 做 YAML 差异对比（myers diff 算法）
- 回滚 = 「选择旧快照 → 以新版本号提交 Apply」（不是 revert 回去，而是向前推进，保留审计轨迹）

### 2.4 Backup Manager + Backup Worker
**位置**：`backend/internal/backup/manager.go` + `worker/backup_worker.go`

#### 任务模型
```
User → HTTP POST /backups
          │
          ▼
 BackupMgr.CreateTask() → Redis Stream: backup_tasks {id, payload...}
          │
          ▼  立即响应 HTTP 202 task_id
          │
 BackupWorker 消费 Redis Stream：
    │
    ├── backup: 拉取对象 YAML → 打包 tar.gz → Storage.Upload()
    │            进度: 80% items_processed + 20% upload_done
    │
    └── restore: Storage.Download() → 解压 → 循环 dynamicClient.Apply()
                 进度: 80% items_processed + 20% finalize

          │
          ▼  Redis PubSub: task_progress:<task_id> → WSHub → Browser
```

#### 恢复模式（restore_mode）
| 模式 | 行为 |
|------|------|
| `overwrite` | 同名对象存在 → `Apply(patch)` 覆盖 |
| `create-new` | 同名对象存在 → 改名 `<name>-restored-<yyyymmdd>` 新建 |

#### 存储后端（插件化）
```go
type IBackupStorage interface {
    Upload(ctx, key, reader) error
    Download(ctx, key) (io.ReadCloser, error)
    Delete(ctx, key) error
    List(ctx, prefix) ([]ObjectInfo, error)
}
```
已实现：Local（文件）/ S3（minio 兼容）/ **NFS**（生产推荐）

### 2.5 Helm Manager
**位置**：`backend/internal/helm/manager.go`

- **ListReleases**：直接查 k8s secrets，筛选 `owner=helm` label，按 `(namespace,name)` 分组取最大 revision
- **Install/Upgrade/Uninstall/Rollback**：写临时 kubeconfig → fork `helm` CLI 执行（避免庞大 Helm Go SDK 依赖）
- 进度通过 `exec.Cmd` stdout 管道实时收集，写到 WS 通道（后续）

### 2.6 WebSocket Hub + 三通道
**位置**：`backend/internal/websocket/hub.go` / `pod_logs.go` / `handler.go`

```
                Browser (3 WebSocket 连接? 不! 实际复用1条)
                        │
               single /ws?token=xxx 升级
                        │
                        ▼
                 ┌──────────┐
                 │   WSHub  │
                 │ Conn Map │conn = { user_id, client_id, subs[channels] }
                 └────┬─────┘
                      │ subscribe(channel)
            ┌─────────┼──────────────────┐
            ▼         ▼                   ▼
    task_progress cluster_event    pod_logs:<cluster>:<ns>:<pod>
   (ring 200)   (ring 200)         (ring 1000)
            │         │                   │
            └─────────┴─────────┬─────────┘
                                │
                      Redis PubSub 广播(多 API Server 间同步)
```

**连接状态机**：
```
   ┌────────── idle
   │  onConnect ▼
   │  ┌─── connecting ───┐
   │  │  (success)       │(fail + retry)
   │  ▼                  └──► reconnecting ───► error
   │  open (on disconnect)
   │  close()
   │  closing ───► closed
   └──────────────────────────
```

### 2.7 RBAC + 审计
**位置**：`backend/internal/auth` + `internal/models` + `api/middleware`

权限点预置 30+（`models/seed.go`）：
- 命名空间模式：`cluster:<action>` / `resource:<action>` / `backup:<action>` / `helm:<action>` ...
- 通配：`*:*`（platform-admin）、`resource:*`（cluster-admin）
- **Scope**：权限范围可以到「指定集群 / 指定命名空间」（`scope_helpers.go`）

**审计中间件**：在每个请求 `after` 阶段写入 `audit_logs` 表：
- user / cluster / namespace / resource_type / name / method
- status_code / latency_ms / client_ip / user_agent
- request_body（超限截断，默认 4KB）

### 2.8 Plugin Manager
**位置**：`backend/internal/plugins/`

插件类型（接口驱动）：
| 类型 | 接口定义 | 已实现 |
|------|----------|--------|
| `Storage` | `IBackupStorage` | Local, S3, NFS |
| `AuthProvider` | `IIdentityProvider` | 本地（JWT）| OIDC（TODO）|
| `KMS` | `IKeyManagement` | Local AES | Vault（TODO）|
| `Notifier` | `INotifier` | 占位 | 钉钉/飞书/Webhook（TODO）|
| `AuditExporter` | `IAuditExporter` | PostgreSQL | Syslog/Splunk（TODO）|

---

## 3. 数据流示例

### 3.1 用户导入集群
```
Browser ─POST /clusters (body: base64 kubeconfig)─► API Server
                                              │
                                              ▼
                                     ClusterMgr.ImportCluster()
                                     ├─ kubeconfig.Parse()
                                     ├─ KMS.AESGCM.Encrypt()
                                     ├─ DB: INSERT clusters
                                     ├─ PingCluster() (初始化连接池)
                                     └─ 审计日志写入
                                              │
                                              ▼
                                       HTTP 201 { cluster }
```

### 3.2 发起命名空间批量备份
```
Browser ─POST /backups (mode=namespace_batch, ns=dev,kinds=Deployment,ConfigMap)
   │
   ▼
API / BackupMgr
  ├─ RBAC: require("backup:create") + scope 校验
  ├─ 生成 Task { id=UUID, payload... }
  ├─ XADD Redis Stream: backup_tasks * id <payload>
  ├─ INSERT backup_tasks 表 status=pending
  └─ HTTP 202 { task_id }
   │
   ▼  Browser WS 订阅 task_progress:<task_id>
   │
Backup Worker ──XREADGROUP backup_tasks──►
  │
  ├─ 拉 ns=dev 下所有 Deployment+ConfigMap (Informer cache)
  │   ├─ 50% ─► Redis PubSub task_progress:X → WSHub broadcast
  ├─ tar.gz YAML 聚合
  ├─ 80% ─► 进度推送
  ├─ StoragePlugin.Upload(key, tar.gz)
  ├─ 100% ─► 进度推送 + status=completed
  └─ INSERT backup_records 表
```

### 3.3 浏览器查看 Pod 实时日志
```
Browser ──────────WS /ws?token=xxx────────► API Server WSHub
│  .send({type:"subscribe", channel:"pod_logs:c1:default:nginx-xx"})
│                                                │
│                                                ▼
│                                      backend/ws handler
│                                      ├─ 鉴权 pod_logs:view
│                                      └─ 启动 k8s GetLogs Stream
│                                          (follow=true, tailLines=500)
│                                                │
◄───────────────────── 每行日志 ring buffer ────┘
   （断线自动重连，重连时请求历史：max(1000 条)）
```

---

## 4. 部署拓扑（推荐）

### 4.1 最小化（本地开发）
```
┌─────────────┐  :5432  ┌───────────┐
│  PostgreSQL  │◄───────│ API Server│◄───► Browser :3000 (Vite dev proxy)
└─────────────┘         └─────┬─────┘
                              │
┌─────────────┐  :6379  ┌─────▼─────┐
│    Redis     │◄───────┤Task Worker│
└─────────────┘         └───────────┘
                              │
                              ▼
                     1..N K8S Clusters
```

### 4.2 生产推荐（HA）
```
                    ┌──────────────┐
                    │   (CDN/WAF)  │
                    └──────┬───────┘
                           ▼
                    ┌──────────────┐
                    │   Ingress    │  TLS termination, 分流 / /api /ws
                    │  (nginx / LB)│
                    └──┬─────┬─────┘
                       │     │
          ┌────────────▼┐   ┌▼───────────────┐
          │ Frontend    │   │  API Server    │───► PostgreSQL (主从 + PITR)
          │ (static nginx)│   │  ×3 HPA       │         │
          └─────────────┘   └────────┬────────┘         │
                                     │                  │
                           ┌─────────▼─────────┐        │
                           │  Redis Cluster    │◄───────┘
                           │ (sentinel / cluster)│
                           └─────────┬─────────┘
                                     │
                           ┌─────────▼─────────┐
                           │   Task Worker     │  ×2..N HPA 按 Redis backlog
                           └─────────┬─────────┘
                                     │  DynamicClient+Informer
                                     ▼
                           ┌───────────────────────┐
                           │ 1..N K8S Clusters      │
                           └───────────────────────┘
```

---

## 5. 扩展点（给插件作者）

### 5.1 新增备份存储后端
```go
// 1. 实现接口（backend/internal/plugins/storage/your_storage.go）
type YourStorage struct{ ... }
func (s *YourStorage) Upload/Download/Delete/List(...)

// 2. 在 PluginMgr 注册
pluginMgr.Register("your_provider", reflect.TypeOf((*IBackupStorage)(nil)).Elem(), yourFactory)

// 3. 在 config.example.yaml 补配置模板
// 4. 在 ROADMAP / CHANGELOG 记录
```

### 5.2 新增权限点
1. `internal/models/seed.go` → `seedPermissionsData` append
2. 路由 `RequirePermission("<code>")`
3. 前端菜单 + 按钮 `usePermission("<code>")`

### 5.3 新增 WebSocket 通道
1. 在 `ws/hub.go` 定义 channel 名 + ring size
2. 消息类型 schema 约定（前端 `wsSlice.ts` 里 reducer）
3. 后端 PubSub 广播对应 topic

---

## 6. 质量与可观测性（TODO：v1.0 GA）

- Prometheus Metrics（`/metrics`）
  - `http_requests_total{route,method,code}` — API 指标
  - `redis_stream_len{name="backup_tasks"}` — 队列积压
  - `informer_active_gvk{cluster}` — 活跃 Informer
  - `backup_duration_seconds{mode,storage}` — 任务耗时
  - `ws_connections_total{channel}` — WS 连接
- 日志字段规范：所有业务日志带 `trace_id`、`cluster_code`、`user_id`
- 告警规则（Grafana AlertManager / PagerDuty）
  - Redis 连接失败 / 主从切换告警
  - API Server 5xx > 1% 持续 5 分钟
  - 备份任务失败累计 > 3 次/小时

---

## 参考

- [设计文档 v1.0](../k8s-platform-system-design.md)（含 MVP 需求矩阵、接口清单）
- [CNCF Architecture Whitepaper](https://github.com/cncf/tag-architecture/blob/main/whitepaper.md)
- [Kubernetes Component Architecture](https://kubernetes.io/docs/concepts/architecture/)
