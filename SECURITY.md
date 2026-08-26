# 安全披露政策（Security Policy）

本文档定义了如何负责任地披露 K8S Platform 项目的安全漏洞，以及维护者如何处理、
修复、公告漏洞。本政策符合 [CNCF 安全披露工作组](https://github.com/cncf/tag-security/tree/main/security-sig)
推荐的流程（Responsible Disclosure / Embargo）。

---

## 报告安全漏洞

> **⚠️ 请不要在公开 Issue / PR / Discussion / 社交媒体 上披露未修复的安全漏洞。**

请通过以下**私密渠道**提交漏洞报告：

### 📧 邮件方式（首选）
直接向维护者/安全响应小组发送邮件（PREFERRED）：
- **主地址**：见 [MAINTAINERS.md](./MAINTAINERS.md) 中各位 Maintainer 的邮箱（任选一位或全部抄送）
- **标题格式**：`[SECURITY] K8S Platform <组件> <严重级别>: <一句话描述>`

邮件中**尽量**包含以下信息（越详细越能加快修复）：
- 漏洞类型（注入 / 越权 / 解密 / 路径穿越 / SSRF / RCE / DoS / 供应链 等）
- 受影响版本（commit hash 或 release tag）
- 最小化复现步骤（越可复现，处理越快）
- CVSS 3.1 向量（可选，或你对严重程度的判断）
- 可能的影响面（认证绕过？集群数据泄露？任意代码执行？）
- 你的 PGP 公钥（如果需要加密通讯；可选）

### 🔒 GitHub 私有安全公告（推荐长期跟进）
1. 打开 [GitHub 仓库 → Security → Advisories → New draft security advisory](https://github.com/Mokaz111/k8s-platform/security/advisories/new)
2. 填写漏洞信息并创建（GitHub 会自动加密，并提供**临时私有 fork** 进行修复协作）
3. 此方式适合复杂漏洞，需要维护者多轮交互 + PR 讨论

### ⏱️ 响应时间

| 漏洞级别 | 初始响应 | 修复目标 |
|----------|----------|----------|
| **Critical**（严重，如 RCE / 全权限绕过） | **24 小时内** | 7 天内 |
| **High**（高危，如鉴权缺陷导致数据泄露） | **48 小时内** | 14 天内 |
| **Medium**（中危，越权/信息泄露非敏感） | 7 天内 | 30 天内 |
| **Low**（低危，防御性增强/极有限场景） | 30 天内 | 下一常规版本 |

> 若超过上述时间未收到回复，请再次发邮件提醒，并抄送其他维护者。
> 本项目在中国大陆/UTC+8 时区，请考虑时差。

---

## 披露与保密协议（Embargo）

K8S Platform 项目遵循 **负责任的披露**（Responsible Disclosure）原则，
要求报告者与维护者双方遵守以下保密期（Embargo）：

| 漏洞严重度 | 默认保密期 | 最长延长 |
|------------|------------|----------|
| Critical   | 90 天      | +30 天（需双方同意） |
| High       | 90 天      | +15 天 |
| Medium     | 60 天      | — |
| Low        | 30 天      | — |

**保密期内：**
- 报告者不得在任何第三方平台公开发布漏洞细节
- 维护者不得在公开 git history 中「悄悄修复」，所有修复必须通过
  **GitHub Private Security Advisory 私有 fork** 完成
- 期满后双方可共同发布公告，或按 `CVE Numbering Authority` 的要求披露

**例外**：如漏洞被恶意方利用或已在公开渠道泄漏，维护者将**立即**发布安全公告 + 补丁。

---

## 严重级别评估（CVSS 3.1 映射）

| 级别 | CVSS 范围 | 典型场景 |
|------|-----------|----------|
| **Critical** | 9.0 - 10.0 | 无需认证远程 RCE；k8s 集群密钥/数据全量泄露；AES key 被导出 |
| **High** | 7.0 - 8.9 | 水平越权访问其他集群备份；WebSocket 消息伪造导致任意命名空间操作；SSRF 访问元数据服务 |
| **Medium** | 4.0 - 6.9 | 登录后垂直越权（user→admin）；备份内容被路径穿越读取；审计日志被注入；NFS 符号链接读取本地文件 |
| **Low** | 0.1 - 3.9 | 反射型 XSS（CSP 已拦截）；前端参数校验缺失；Debug 日志敏感字段未脱敏 |

---

## 漏洞修复流程

```
                   报告者                                 维护者
                     │                                      │
  1. 报告漏洞 ───────►                                      │
                     │                                      │
                     │         2. 分配置严重度/CVE ──────►  │
                     │                                      │
                     │     3. 创建 GitHub Private Advisory  │
                     │        (可邀请报告者加入评审)         │
                     │                                      │
                     │    4. 私有 fork 开发修复 PR          │
                     │    5. 报告者复实验证修复              │
                     │                                      │
                     │    6. 保密期内：通知分发列表          │
                     │         （大型 adopters 预取补丁）     │
                     │                                      │
                     │    7. 保密期满：合并到 main          │
                     │    8. 打 Tag + 发 GitHub Release     │
                     │    9. 发布 Security Advisory + CVE   │
                     │                                      │
  10. 致谢 Credit ◄──────────────────────────────────────── │
```

---

## 安全公告内容

每一个修复的漏洞都会以 **GitHub Security Advisory** 形式发布，包含：
- CVE 编号（如已申请）
- 受影响版本范围 + 修复版本
- 简短描述 + 临时缓解措施（若无法立即升级）
- 致谢报告者（可选择匿名/署名）
- 引用 Commit / PR 链接

查看所有已发布公告：  
👉 <https://github.com/Mokaz111/k8s-platform/security/advisories>

---

## 威胁模型（当前范围）

了解我们当前的安全边界可以帮助你更准确地判断问题严重度：

### 受信任边界
- **API Server** 后端进程与 **Task Worker** 进程之间为「可信通道」（本地 DB + Redis）
- 已登录且通过 RBAC 鉴权的用户 → Web UI 操作视为「内部」操作
- 集群 kubeconfig / 数据库 / KMS key 泄漏被视为「环境级安全事件」，非本项目边界内

### 范围内的攻击面
- `/api/v1` 未认证接口（JWT 伪造、暴力破解）
- WebSocket 三条通道（Pod 日志流 / 任务进度 / 集群事件）消息伪造
- 备份后端文件写入（路径穿越 / 符号链接）
- KMS 加解密实现（AES-GCM nonce 重用、IV 预测）
- RBAC 权限绕过（permission scope 不生效）
- 前端模板注入 / XSS（Ant Design Pro 的表单渲染）
- Helm values YAML 编辑（命令注入 into helm CLI）
- kubeconfig 导入解析（证书文件解析器 RCE / XXE）

### 范围外 / 不视为安全漏洞
- 管理员故意泄露 JWT secret → 非代码层面
- 用户使用了示例配置中的默认密码（`please-change-me`）→ 已高亮警告
- K8S 集群本身的漏洞（如 etcd 未加密、API Server 匿名访问）
- 已知被标记为 `WONTFIX` 或 `EXPECTED` 的行为（请先查 Issue/Discussion）
- DoS 只考虑**大规模、无需认证**即可触发；单纯大请求体被 nginx 拒绝不属于漏洞

---

## Bug Bounty（赏金）

> 项目早期阶段不设正式赏金计划，但所有在**首次报告**中指出 Critical/High
> 级别漏洞并协助完成修复的独立研究员，我们会：
> - 在发布公告中 **专门致谢 + 链接你的 GitHub/GPG Key**
> - 邀请加入 `SECURITY-ACK.md` 感谢名单
> - 项目进入 CNCF Sandbox 后优先纳入 Bug Bounty 白名单

如需赏金请在报告中提前注明，我们会尽力协调（但不做承诺）。

---

## 相关文档

- [Maintainers List](./MAINTAINERS.md) — 报告联系人
- [Contributing Guide](./CONTRIBUTING.md) — 贡献/审核规范
- [LICENSE](./LICENSE) — Apache-2.0 协议免责声明
- [CNCF TAG Security Vulnerability Disclosure Best Practices](https://github.com/cncf/tag-security/blob/main/security-sig/charter.md)
