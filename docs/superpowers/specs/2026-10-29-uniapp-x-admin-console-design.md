# DeepNow uni-app x 管理控制台规格

**日期：** 2026-10-29  
**状态：** 已确认设计，待实现计划

## 1. 目标

将当前 DeepNow 的静态管理页迁移为 uni-app x 管理控制台，首期只交付 H5，保留现有 Go 网关和数据库作为后端基础。前端先独立开发和构建，后续在功能稳定后将 H5 产物嵌入 Go 二进制。

首期重点是让管理员可以完成模型、API Key、RAG、规则和系统配置的完整管理，并获得清晰的运行状态、诊断和错误反馈。

## 2. 范围

### 2.1 P0：迁移必须可用

- 登录、退出、会话过期处理
- 系统运行概览
- 主模型、备用模型、Embedding 模型配置和激活
- API Key 生成、导入、删除、使用量查看
- RAG 文件上传和索引状态
- RAG 规则读取和保存
- 系统配置读取和保存
- 全局加载、错误、空状态、未授权状态

### 2.2 P1：首期新增

- 模型连通性测试，不改变激活状态
- 配置校验，检查地址、端口、模型、TLS 路径
- Key 名称、备注、状态、创建时间、最近使用时间
- 请求数、输入 Token、输出 Token、错误数统计
- RAG 文档列表、大小、分块数、索引状态和删除
- 网关、数据库、Embedding 和模型健康诊断
- 登录、配置、模型、Key、文档操作审计

### 2.3 P2：只做接口和导航预留

- 多用户、角色和权限
- Key 配额、限流、计费插件
- 短期记忆和跨客户端会话
- 多模态 RAG
- 模型权重编排和 next-hop 接管
- Agent 事务任务

P2 在后端没有真实能力之前不显示为可操作功能，不使用假数据冒充已实现能力。

## 3. 非目标

- 首期不开发普通用户聊天端
- 首期不做 Android 或 iOS 原生适配
- 首期不实现流式聊天调试页面
- 不重写现有 Go 网关的协议转发逻辑
- 不替换 SQLite 数据库方案
- 不一次性重命名现有 API 路径

## 4. 技术方案

### 4.1 工程边界

新增独立 `frontend/` uni-app x 工程，使用 TypeScript 编写页面、请求层、状态层和组件。开发环境前端独立运行，使用配置的 API 基地址访问 Go 管理 API。

生产交付分两步：

1. 首期将 H5 构建目录作为静态资源部署，验证页面和 API。
2. H5 稳定后，使用 Go `embed` 将构建资源嵌入服务端，恢复单文件交付能力。

### 4.2 前端层次

```text
frontend/
├── pages/
├── components/
├── layouts/
├── services/
├── stores/
├── types/
└── utils/
```

- `pages/`：路由页面和页面级数据加载
- `components/`：状态、列表、表单、弹窗等通用组件
- `layouts/`：桌面侧栏、移动底部导航和顶部工具栏
- `services/`：`request()`、`upload()` 及按领域拆分的 API
- `stores/`：会话、系统状态和全局通知
- `types/`：响应、配置、模型、Key、RAG 文档和诊断类型
- `utils/`：错误映射、格式化和响应式工具

页面不得直接调用 `uni.request`。所有网络访问必须经过请求层，保证错误处理、会话失效和后续 App Authorization 兼容性一致。

### 4.3 会话

当前 H5 使用后端 HttpOnly Cookie 会话：

- `POST /api/login` 成功后由服务端设置 Cookie
- 请求层默认携带 Cookie
- `401` 统一清理前端会话并跳转登录页
- `POST /api/logout` 清理服务端会话

请求层保留可选的 Authorization 注入点，但首期不在 UI 中引入第二套登录流程。

## 5. 后端接口契约

### 5.1 统一响应

成功和业务错误统一使用：

```json
{
  "success": true,
  "message": "操作成功",
  "data": {},
  "request_id": "req_20261029_xxx"
}
```

HTTP 状态码约定：

- `401`：未登录或会话过期
- `403`：无权限
- `400`：参数校验失败
- `409`：资源冲突
- `502`：上游模型失败
- `500`：服务端错误

### 5.2 接口清单

认证和会话：

```text
POST /api/login
POST /api/logout
GET  /api/session
```

系统和配置：

```text
GET  /api/system/status
GET  /api/system/build
GET  /api/config/get
POST /api/config/save
GET  /api/diagnostics
```

模型：

```text
GET  /api/models/get
POST /api/models/save
POST /api/models/activate
POST /api/models/test
```

访问控制：

```text
GET  /api/keys/list
POST /api/keys/generate
POST /api/keys/import
POST /api/keys/delete
```

RAG：

```text
GET    /api/rag/status
GET    /api/rag/documents
POST   /api/rag/upload
DELETE /api/rag/documents/{id}
POST   /api/rag/test
```

规则和审计：

```text
GET  /api/rules/get
POST /api/rules/save
GET  /api/audit/events
```

已有路径优先保持兼容，新增接口采用增量方式实现。

### 5.3 返回数据约束

模型接口不得返回上游 API Key 原文：

```json
{
  "provider": "openai",
  "api_type": "LM",
  "model_name": "model-name",
  "endpoint": "https://api.example.com",
  "api_key_masked": "sk-****abcd",
  "active_role": "primary",
  "healthy": true
}
```

Key 接口返回：

```json
{
  "id": "key-id",
  "key": "sk-aibase-****",
  "name": "研发环境",
  "status": "active",
  "request_count": 42,
  "input_tokens": 0,
  "output_tokens": 0,
  "error_count": 0,
  "created_at": "2026-10-29T10:00:00Z",
  "last_used_at": null
}
```

列表和详情接口只返回脱敏 Key。批量生成接口可以在成功响应中返回新 Key 原文一次，前端必须提示管理员立即保存，后续任何查询接口都不得再次返回原文。

RAG 文档接口返回：

```json
{
  "id": 12,
  "filename": "manual.md",
  "size": 20480,
  "chunk_count": 18,
  "status": "ready",
  "error": "",
  "created_at": "2026-10-29T10:00:00Z"
}
```

## 6. 数据库改造

现有表保持兼容，按迁移脚本或启动时幂等迁移扩展：

- `access_keys`：增加 `id`、`name`、`status`、`last_used_at`
- `key_usage_daily`：按 Key 和日期保存请求数、Token 和错误数
- `knowledge_documents`：保存源文件、大小、分块数和索引状态
- `audit_events`：保存管理员操作、资源类型、资源 ID、结果和时间
- `model_configs`：继续保存上游配置，接口层只提供脱敏字段

`/api/system/status`、`/api/models/get` 和概览页使用的所有模型状态数据都必须经过同一套脱敏转换，不能直接序列化包含 `ApiKey` 的内部 `ModelConfig`。

所有迁移必须可重复执行，已有数据库升级失败时不得静默破坏原数据。

## 7. 页面信息架构

```text
概览
├── 运行状态
├── 模型健康度
├── Key 使用摘要
└── RAG 索引摘要

模型
├── 模型列表
├── 模型配置
├── 主备激活
└── 连通性测试

知识库
├── 文档列表
├── 上传文档
├── 索引状态
└── 检索测试

访问控制
├── API Key
├── 使用统计
└── 操作审计

策略
├── RAG 策略
├── 路由与主备
└── 并发与超时

系统
├── 网络与 TLS
├── 诊断信息
└── 管理员设置
```

## 8. 响应式布局

### 8.1 桌面

- `>= 1200px`：固定 `224px` 侧栏、顶部工具栏、主内容最大宽度 `1440px`
- `768px - 1199px`：窄侧栏、两列信息区
- 页面结构统一为标题区、工具栏、数据区
- 列表操作放在行右侧，详情和编辑使用抽屉或独立详情页
- 概览使用紧凑指标带、状态列表和最近事件，不使用嵌套卡片

### 8.2 移动 H5

- `< 768px`：底部导航显示概览、模型、知识库、更多
- 访问控制、策略和系统放入更多菜单
- 桌面表格改为纵向信息行，操作菜单放在右侧
- 配置页面使用分组表单，保存操作固定在底部安全区域
- 上传、模型测试和诊断结果使用全屏页或底部面板
- 不将桌面侧栏简单压缩到手机宽度，不使用手机样机嵌套展示

### 8.3 视觉和交互

- 浅色工作区、深色文字、蓝色主操作色
- 绿色、黄色、红色只用于表达状态
- 圆角不超过 `6px`
- 高信息密度，减少装饰性留白
- 所有按钮和列表操作支持加载、成功、失败、禁用态
- 表单支持键盘焦点、触控区域和窄屏换行

## 9. 验收标准

### 9.1 功能

- 未登录访问受保护页面会跳转登录
- 登录失败显示后端错误，不泄露敏感信息
- 登录成功后可以读取系统状态
- 模型保存、测试和激活结果可见且不混淆
- Key 生成、导入、删除后列表立即刷新
- API Key 和上游模型 Key 均不在 UI 中展示完整秘密值；新生成的 Key 只在生成成功结果中展示一次
- RAG 上传显示成功、失败和处理中状态
- 配置保存失败时保留用户输入并显示原因
- `401` 会话过期时所有页面行为一致

### 9.2 兼容性

- H5 桌面宽度 `1440px`、`1024px` 正常布局
- H5 移动宽度 `390px`、`768px` 无横向滚动和文字重叠
- API 层错误不会导致未处理 Promise 或白屏
- 现有 Go 测试继续通过
- 新增前端请求、状态和关键交互均有单元测试
- 关键页面有最少一组端到端流程测试：登录、模型配置、Key 操作、RAG 上传

## 10. 实施顺序

1. 创建 uni-app x 前端工程和请求层
2. 补齐会话、统一响应和后端基础接口
3. 实现 AppShell、登录和概览
4. 实现模型、Key 和 RAG 页面
5. 实现策略、系统和诊断页面
6. 增加 P1 的统计、连通性测试和审计
7. 完成桌面和移动 H5 验证
8. 评估将 H5 构建资源嵌入 Go
