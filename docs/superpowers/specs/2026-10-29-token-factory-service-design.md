# DeepNow Token Factory 独立服务设计

**日期：** 2026-10-29  
**来源：** 飞书文档《Token工厂服务设计》  
**状态：** 已确认设计，待实现计划

## 1. 目标与边界

将 Token Factory 建设为当前仓库中的独立 Go 模块化单体服务。它拥有独立 API、配置、MySQL 数据库和 Redis 命名空间；当前 DeepNow 网关作为后续 Adapter 接入方，不把 Token Factory 核心逻辑继续堆入现有网关。

首期目标：

- 统一管理应用和 Token 元数据
- 支持 API Token、Service Token、Temporary Token、LLM Gateway Token
- 支持申请、审批、签发、查询、轮换、吊销、延期
- Secret 加密保存，创建和轮换时仅展示一次
- 支持 Scope、环境、IP 白名单、QPS、日额度
- 支持 Token 校验、Redis 缓存、黑名单和用量统计
- 支持审计日志、基础风险事件和管理员控制台
- 为 Vault/KMS、事件总线、通知、DeepNow Gateway Adapter 预留接口

非目标：

- 首期不拆分多个独立微服务
- 首期不依赖 Vault/KMS 才能运行
- 首期不实现完整企业审批平台和复杂机器学习风控
- 首期不删除或重构当前 `frontend/` uni-app x 控制台
- 首期只删除旧 `static/` 简易前端
- 不把真实数据库密码、Secret 或 Token 写入代码、文档、测试数据

## 2. 当前仓库处理

### 2.1 保留内容

- `frontend/`：保留，作为 Token Factory 管理控制台基础
- 当前 Go DeepNow 网关：保留，后续通过 Adapter 接入 Token 校验
- `assets/`、`doc/`、`README*`：保留，后续按服务边界补充文档
- 当前已有 Go 测试和前端测试：保留并持续回归

### 2.2 删除内容

- 删除 `static/` 目录及其中旧的简易登录页面
- 删除 Go 中对旧 `static/` 文件服务器的依赖
- Go 管理 API 改由独立 Token Factory 服务提供
- 当前 DeepNow 嵌入的旧前端资源不再作为 Token Factory 入口

删除动作只针对旧 `static/`，不删除 `frontend/` 和已有测试。

## 3. 服务架构

### 3.1 目录结构

```text
token-factory/
├── cmd/
│   └── token-factory/
│       └── main.go
├── internal/
│   ├── app/
│   ├── application/
│   ├── token/
│   ├── policy/
│   ├── approval/
│   ├── validation/
│   ├── usage/
│   ├── audit/
│   ├── risk/
│   ├── notification/
│   └── infrastructure/
│       ├── mysql/
│       ├── redis/
│       ├── crypto/
│       └── http/
├── migrations/
├── api/
│   └── openapi.yaml
├── tests/
└── README.md
```

服务职责：

- `application`：应用和 Owner 管理
- `token`：Token 元数据、版本和生命周期
- `policy`：Scope、环境、额度和风险规则
- `approval`：审批请求、节点、转交、加签和状态流转
- `validation`：Token Hash、状态、有效期、Scope、IP 和额度校验
- `usage`：调用次数、Token 量和日统计
- `audit`：所有敏感操作留痕
- `risk`：风险事件、评分和基础处置
- `notification`：飞书、邮件、Webhook 的适配接口
- `infrastructure`：MySQL、Redis、加密和 HTTP 实现

### 3.2 端口与服务关系

```text
uni-app x Console  ----\
OpenAPI / SDK / CLI ----- > Token Factory API :8090
DeepNow Gateway -------/

Token Factory API --> MySQL :3306 / database token_factory
Token Factory API --> Redis :6379 / key prefix tf:
```

当前 Docker 中已有 `mysql8` 和 `redis` 容器时直接复用，不重建、不删除已有数据。MySQL 连接密码只从本地环境变量或未提交的 `.env.local` 读取。

建议环境变量：

```text
TOKEN_FACTORY_HTTP_ADDR=:8090
TOKEN_FACTORY_MYSQL_DSN=...
TOKEN_FACTORY_REDIS_ADDR=127.0.0.1:6379
TOKEN_FACTORY_REDIS_PREFIX=tf:
TOKEN_FACTORY_SECRET_KEY=...
TOKEN_FACTORY_ADMIN_SESSION_TTL=8h
```

### 3.3 数据流

同步路径：

```text
申请 -> Policy Engine -> 审批判断 -> 签发 -> 加密保存 -> 一次性返回 Secret
```

校验路径：

```text
调用方 -> Validation Service
       -> Redis Token Cache
       -> MySQL 回源
       -> Scope/IP/环境/额度判断
       -> Allow/Deny
       -> 异步写 Usage/Audit/Risk
```

治理路径：

```text
Usage/Audit -> Risk Engine -> 风险事件
            -> 通知 / 限流 / 冻结 / 吊销
```

## 4. Token 类型与状态

### 4.1 Token 类型

| 类型 | 用途 | 默认策略 |
|---|---|---|
| `api_token` | 内部或外部 API 调用 | 支持 Scope、IP 白名单、额度和轮换 |
| `service_token` | 服务到服务调用 | 默认禁止人工重复查看，短周期轮换 |
| `temporary_token` | 调试、临时任务、一次性脚本 | 短有效期、低权限、自动过期 |
| `llm_gateway_token` | DeepNow 或其他 AI 网关调用 | 绑定模型范围、Token 额度和内容安全策略 |

### 4.2 生命周期状态

```text
Draft
  -> PendingApproval
  -> Rejected
  -> Active
  -> Rotating
  -> Expired
  -> Revoked
  -> RotationFailed
```

状态规则：

- `Draft`：仅申请人可见
- `PendingApproval`：等待全部必需节点处理
- `Rejected`：申请结束，可复制为新申请
- `Active`：允许校验和调用
- `Rotating`：新旧版本按灰度期同时有效
- `Expired`：超过有效期，禁止调用
- `Revoked`：主动吊销，立即加入黑名单
- `RotationFailed`：轮换未完成，不自动吊销当前有效版本

## 5. 审批设计

### 5.1 自动审批

同时满足以下条件时可自动通过：

- Dev/Test 环境
- 只读 Scope
- 短有效期
- 低额度
- 非敏感数据
- 申请人属于应用成员
- 非 Service Token 生产用途

### 5.2 人工审批

以下条件至少命中一项时进入审批：

- Prod 环境
- 写入、删除或管理权限
- 高敏数据 Scope
- 跨部门资源访问
- 长有效期
- 高 QPS 或高日额度
- Service Token
- 高额度 `llm_gateway_token`

审批链：

```text
低风险 Dev/Test         -> 自动审批
生产 Token               -> Application Owner
高敏 Scope               -> Application Owner -> Security Reviewer
跨部门资源               -> Application Owner -> Resource Owner
高额度 Token              -> Application Owner -> Platform Admin
```

审批动作：

```text
Approve / Reject / Return / Transfer / AddApprover
```

审批请求必须保存申请人、应用、Scope、环境、有效期、额度、风险结果、节点、意见、处理人和时间。

## 6. Secret 与安全设计

- 数据库只保存 `secret_ciphertext` 和不可逆 `secret_hash`
- 首期使用应用层 AES-GCM，密钥从 `TOKEN_FACTORY_SECRET_KEY` 读取
- 后续通过 `SecretStore` 接口接入 Vault/KMS
- Secret 只在创建和轮换成功响应中返回一次
- 列表、详情、审计、错误响应不返回 Secret
- 高风险 Service Token 默认禁止人工查看
- 前端不将 Secret 写入 localStorage、IndexedDB 或日志
- 复制 Secret 必须写审计事件
- 日志中只允许 Token ID 或脱敏指纹
- 吊销时同步更新 MySQL、Redis 黑名单和本地缓存

首期使用不透明 Token：

```text
tf_live_<random-secret>
```

校验通过 Hash 查找元数据和版本。JWT 作为未来边缘场景适配，不纳入首期。

## 7. MySQL 设计

### 7.1 数据库

```sql
CREATE DATABASE token_factory
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;
```

### 7.2 应用表

```sql
CREATE TABLE applications (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  app_id VARCHAR(64) NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  description TEXT NOT NULL,
  owner_id VARCHAR(128) NOT NULL,
  department_id VARCHAR(128) NOT NULL,
  env VARCHAR(16) NOT NULL,
  data_level VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  deleted_at DATETIME(6) NULL,
  INDEX idx_app_owner (owner_id),
  INDEX idx_app_department (department_id)
);
```

### 7.3 Token 表

```sql
CREATE TABLE tokens (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  token_id VARCHAR(64) NOT NULL UNIQUE,
  app_id VARCHAR(64) NOT NULL,
  token_type VARCHAR(32) NOT NULL,
  name VARCHAR(128) NOT NULL,
  status VARCHAR(32) NOT NULL,
  env VARCHAR(16) NOT NULL,
  scopes JSON NOT NULL,
  quota JSON NOT NULL,
  expires_at DATETIME(6) NOT NULL,
  created_by VARCHAR(128) NOT NULL,
  owner_id VARCHAR(128) NOT NULL,
  risk_level VARCHAR(16) NOT NULL,
  rotate_after DATETIME(6) NULL,
  last_used_at DATETIME(6) NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  deleted_at DATETIME(6) NULL,
  INDEX idx_tokens_app_status (app_id, status),
  INDEX idx_tokens_expires (expires_at),
  INDEX idx_tokens_risk (risk_level)
);
```

### 7.4 Token 版本表

```sql
CREATE TABLE token_versions (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  token_id VARCHAR(64) NOT NULL,
  version INT NOT NULL,
  secret_ciphertext VARBINARY(4096) NOT NULL,
  secret_hash CHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  valid_from DATETIME(6) NOT NULL,
  valid_until DATETIME(6) NULL,
  created_at DATETIME(6) NOT NULL,
  UNIQUE KEY uk_token_version (token_id, version),
  INDEX idx_secret_hash (secret_hash)
);
```

### 7.5 审批与审计表

```sql
CREATE TABLE approval_requests (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  request_id VARCHAR(64) NOT NULL UNIQUE,
  request_type VARCHAR(32) NOT NULL,
  token_id VARCHAR(64) NULL,
  app_id VARCHAR(64) NOT NULL,
  applicant_id VARCHAR(128) NOT NULL,
  payload JSON NOT NULL,
  risk_level VARCHAR(16) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  INDEX idx_approval_status (status, created_at)
);

CREATE TABLE approval_steps (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  request_id VARCHAR(64) NOT NULL,
  step_order INT NOT NULL,
  approver_type VARCHAR(32) NOT NULL,
  approver_id VARCHAR(128) NULL,
  status VARCHAR(32) NOT NULL,
  comment TEXT NOT NULL,
  handled_at DATETIME(6) NULL,
  UNIQUE KEY uk_approval_step (request_id, step_order)
);

CREATE TABLE audit_logs (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  request_id VARCHAR(64) NOT NULL,
  actor_id VARCHAR(128) NOT NULL,
  actor_type VARCHAR(32) NOT NULL,
  action VARCHAR(64) NOT NULL,
  resource_type VARCHAR(32) NOT NULL,
  resource_id VARCHAR(128) NOT NULL,
  before_json JSON NULL,
  after_json JSON NULL,
  result VARCHAR(16) NOT NULL,
  source_ip VARCHAR(64) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  INDEX idx_audit_resource (resource_type, resource_id),
  INDEX idx_audit_created (created_at)
);
```

### 7.6 用量与风险表

```text
policies
token_usage_daily
risk_events
notification_records
```

调用明细不进入 MySQL 主表，优先进入后续日志/分析系统；首期只保存日聚合。

## 8. API 设计

### 8.1 统一响应

```json
{
  "request_id": "req_xxx",
  "success": true,
  "message": "",
  "data": {}
}
```

HTTP 状态：

```text
400 参数错误
401 未认证
403 无权限
404 资源不存在
409 幂等冲突或状态冲突
422 审批/策略拒绝
429 超额或限流
500 服务错误
503 依赖不可用
```

### 8.2 核心接口

```text
POST /api/v1/apps
GET  /api/v1/apps
GET  /api/v1/apps/{app_id}
PATCH /api/v1/apps/{app_id}

POST /api/v1/tokens
GET  /api/v1/tokens
GET  /api/v1/tokens/{token_id}
POST /api/v1/tokens/{token_id}/rotate
POST /api/v1/tokens/{token_id}/revoke
POST /api/v1/tokens/{token_id}/extend
GET  /api/v1/tokens/{token_id}/versions
GET  /api/v1/tokens/{token_id}/usage

POST /api/v1/tokens/validate

GET  /api/v1/approvals
GET  /api/v1/approvals/{request_id}
POST /api/v1/approvals/{request_id}/approve
POST /api/v1/approvals/{request_id}/reject
POST /api/v1/approvals/{request_id}/return

GET  /api/v1/audit-logs
GET  /api/v1/risk-events
POST /api/v1/risk-events/{event_id}/resolve

GET  /api/v1/policies
PUT  /api/v1/policies/{policy_id}
```

所有写操作支持 `Idempotency-Key`；创建和轮换响应可返回一次性 `secret`，其他响应禁止返回。

### 8.3 创建 Token 示例

```json
{
  "app_id": "app_123",
  "name": "order-service-prod",
  "token_type": "service_token",
  "env": "prod",
  "scopes": ["order.read", "order.write"],
  "expires_at": "2026-12-31T23:59:59Z",
  "ip_whitelist": ["10.1.0.0/16"],
  "quota": {
    "qps": 100,
    "daily_limit": 1000000
  },
  "reason": "订单服务调用支付服务"
}
```

自动签发：

```json
{
  "request_id": "req_123",
  "token_id": "tok_123",
  "status": "active",
  "risk_level": "medium",
  "secret": "只展示一次的 secret",
  "expires_at": "2026-12-31T23:59:59Z"
}
```

需审批：

```json
{
  "request_id": "req_123",
  "status": "pending_approval",
  "risk_level": "high"
}
```

## 9. Redis 操作契约

```text
tf:token:cache:{token_hash}
TTL: 60s
内容: token_id, version, status, scopes, app_id, expires_at

tf:token:blacklist:{token_hash}
TTL: Token 最晚失效时间
内容: revoke_reason

tf:quota:{token_id}:{yyyyMMdd}
命令: INCRBY
TTL: 48h

tf:rate:{token_id}:{window}
命令: INCR
TTL: 1-60s

tf:lock:token:{token_id}
命令: SET NX EX
用途: 轮换、吊销、延期互斥

tf:approval:{request_id}
TTL: 24h
内容: 审批节点快照
```

失败策略：

- 校验缓存失败：回源 MySQL，不能默认放行
- 黑名单查询失败：生产或高风险 Token 默认拒绝
- 额度计数失败：按策略拒绝或受限降级
- 分布式锁失败：返回 `409`，不重复签发
- Redis 数据全部可重建，MySQL 是事实源

## 10. uni-app x 控制台

保留现有 `frontend/`，将页面从 DeepNow 管理控制台演进为 Token Factory 控制台。

一级导航：

```text
概览
应用
Token
审批中心
风险中心
审计日志
策略配置
接入文档
```

首期页面：

- Dashboard：Token 总数、活跃数、即将过期、高风险、今日签发和调用
- 应用列表/详情/创建
- Token 列表：应用、状态、环境、Scope、风险、过期时间筛选
- Token 申请：基础信息、权限、额度、审批预览
- Token 详情：Metadata、版本、使用统计、轮换、吊销
- 审批中心：同意、拒绝、退回、转交、加签
- 审计日志：操作人、动作、资源、结果、请求 ID、来源 IP
- 风险中心：长期未轮换、过期调用、异常 IP、疑似泄露
- 策略配置：Token 类型、审批、轮换、额度和通知

前端规则：

- Secret 使用一次性展示组件
- 高危操作必须二次确认
- 权限不足的按钮不渲染
- 列表默认只显示脱敏 Token
- 所有 API 调用经过统一 `request()` 层
- H5 和未来 App 共享业务服务与类型

## 11. 子 agent 分工

### Agent A：基础工程与启动

范围：

- `token-factory/cmd/token-factory`
- 配置加载
- HTTP Server
- MySQL/Redis 连接
- 健康检查
- Docker 连接文档

依赖：无。  
验收：独立服务可启动，连接现有 MySQL/Redis，`/healthz` 返回依赖状态。

### Agent B：MySQL 模型与迁移

范围：

- `migrations/`
- MySQL Repository
- applications、tokens、token_versions、approvals、audit 表
- 迁移测试

依赖：API/DDL 设计。  
验收：迁移幂等，数据库不保存明文 Secret。

### Agent C：Redis 适配

范围：

- Token Cache
- 黑名单
- 额度计数
- 限流
- 分布式锁

依赖：Redis 操作契约。  
验收：TTL、前缀、失败策略和并发测试完整。

### Agent D：Token 生命周期

范围：

- Token 创建
- Secret 加密/Hash
- 一次性返回
- 查询
- 轮换
- 吊销
- 延期

依赖：B、C 的接口。  
验收：状态机、幂等、并发轮换和 Secret 保护测试通过。

### Agent E：Policy 与 Approval

范围：

- 风险条件计算
- 自动审批
- 多节点审批
- 同意、拒绝、退回、转交、加签
- 审批审计

依赖：D 的领域接口。  
验收：所有审批策略有表驱动测试。

### Agent F：Validation 与 Usage

范围：

- Token Hash 校验
- Scope、IP、环境和状态检查
- QPS/日额度
- 使用量聚合

依赖：B、C、D。  
验收：允许/拒绝原因稳定，Redis 异常不绕过安全校验。

### Agent G：Audit 与 Risk

范围：

- 审计事件
- 基础风险评分
- 风险事件查询和处置
- 通知接口抽象

依赖：事件模型和 D/F。  
验收：创建、查看、复制、轮换、吊销、校验、审批均有审计。

### Agent H：uni-app x 控制台

范围：

- `frontend/src/pages`
- Token Factory API services
- 应用、Token、审批、风险、审计页面
- 一次性 Secret、权限级渲染、响应式布局

依赖：OpenAPI 契约，可先使用 Mock。  
验收：H5 桌面/移动流程通过，前端不保存明文 Secret。

### Agent I：DeepNow Adapter

范围：

- `internal/adapter/deepnow`
- Gateway Token 校验
- Scope/额度映射
- 调用量上报

依赖：F 的 Validation/Usage 契约。  
验收：DeepNow 可选择性接入，不影响原有网关请求。

### Agent J：集成、测试与部署

范围：

- Docker 连接验证
- API/前端/数据库集成测试
- 端到端流程
- README、运行脚本和安全检查

依赖：A-I。  
验收：一条命令启动 Token Factory，完整走通申请→审批→签发→校验→轮换→吊销。

执行依赖：

```text
A
├── B
└── C

B + C
├── D
├── F
└── G

D
└── E

API 契约
└── H

F + D
└── I

A-I
└── J
```

每个 agent 必须遵守：

1. 先写失败测试，再写实现。
2. 只修改职责范围内的文件。
3. 不写真实密码、Secret 或生产数据。
4. 每个任务单独 commit。
5. 汇报文件、测试命令、结果和遗留风险。

## 12. 测试策略

### 后端

```text
单元测试：状态机、策略、加密、Scope、额度、Redis 错误
Repository 测试：MySQL 迁移、幂等、事务
API 测试：请求/响应、错误码、鉴权、幂等
并发测试：轮换、吊销、额度、锁
安全测试：Secret 泄露、越权、重放、日志脱敏
```

### 前端

```text
请求层和状态层单测
一次性 Secret 展示/清理测试
权限级渲染测试
Token 列表筛选测试
Playwright：登录、申请、审批、详情、轮换、吊销
Playwright：1440、1024、390 视口
```

### 集成验收

```text
创建应用
申请低风险 Token 自动通过
申请生产 Token 进入审批
审批通过并返回一次性 Secret
校验成功并扣减额度
轮换时旧版本灰度有效
吊销后 Redis 黑名单立即生效
审计日志覆盖所有关键动作
Redis 故障不会绕过安全校验
```

## 13. 发布和迁移策略

### 阶段 0：基础设施

- 复用现有 MySQL/Redis 容器
- 创建 `token_factory` 数据库
- 配置环境变量
- 执行迁移
- 启动 `/healthz`

### 阶段 1：独立 Token Factory MVP

- 应用管理
- Token 签发、查询、轮换、吊销
- 基础审批
- 审计
- Validation API
- uni-app x 控制台

### 阶段 2：DeepNow Adapter

- 将现有 DeepNow API Key 映射为 Token 资源
- 逐步接入 Scope、额度和审计
- 保留兼容开关
- 默认关闭 Adapter，验证后按应用开启

### 阶段 3：安全治理

- 风险中心
- 自动轮换
- 泄露扫描
- 通知和工单
- Vault/KMS
- 多区域和事件总线

上线原则：

- 新服务独立端口运行
- 不修改现有 SQLite 数据
- 不删除当前网关数据
- 先旁路校验和记录，再启用强制拒绝
- 所有强制策略支持配置回滚

## 14. 成功标准

- Token Factory 可独立启动并连接现有 MySQL/Redis
- Secret 不以明文落库、落日志或二次展示
- 申请、审批、签发、校验、轮换、吊销闭环可测试
- 生产 Token 默认有 Owner、Scope、有效期、额度和审计
- Redis 故障不会导致未授权放行
- uni-app x 控制台覆盖 MVP 页面并通过桌面/移动 H5 验证
- DeepNow Adapter 可插拔，不破坏现有网关能力
- 所有 P0 功能和关键安全规则都有自动化测试

