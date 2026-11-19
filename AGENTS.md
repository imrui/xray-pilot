# AGENTS.md — AI 协作与工程上下文指南

本文件是 `xray-pilot` 仓库面向各类 AI 编码工具（Claude Code、Codex、Antigravity、Cursor 等）的统一工程上下文与行为准则。

- 产品功能、部署方式、环境变量与配置示例见 [README.md](./README.md) 与 [README.zh-CN.md](./README.zh-CN.md)。
- 各大版本架构演进、技术取舍与在研方案见 [docs/design/README.md](./docs/design/README.md)。
- 版本发布说明写作规范见 [release-notes/README.md](./release-notes/README.md)。

---

## 1. 项目定位与技术栈速览

- **定位**：小团队/自用多节点（约 12 台公网 VPS）Xray 管理面板，单二进制内嵌前端分发。
- **后端**：Go 1.26+ · Gin · GORM · SQLite（默认，单连接 + `busy_timeout`） / PostgreSQL（可选） · JWT · Zap · AES-GCM。
- **前端**：React 19 · Vite 8 · TypeScript · Tailwind CSS v4 · Zustand · TanStack Query · Radix UI，`frontend/dist` 经 `go:embed` 嵌入二进制。
- **节点控制面**：SSH 推送完整配置（远端预检 + 同目录原子替换 + 健康探测回滚） + SSH 隧道内访问本地 `127.0.0.1:10085` gRPC API（`HandlerService` 用户热增删、`StatsService` 流量采集）。不做常驻节点 Agent。

---

## 2. 核心领域模型与系统不变量（严禁破坏）

1. **身份隔离**：
   - `AdminUser`（登录后台的管理员）：两级硬编码角色 `super_admin` / `admin`（不上 RBAC、不做 TOTP）。`config.yaml` 的 `admins` 仅在空表首次启动时作种子，之后以 DB 为准。中间件每请求按 `admin_id` 回查 `Active`（不做 token 黑名单）；护栏禁止删除/降级/禁用最后一名启用的 `super_admin` 或操作自身角色/状态。
   - `User`（订阅用户）：无密码，凭 `Token` 匿名访问 `/sub/:token` 获取订阅（Base64 / Clash / sing-box / 浏览器信息页）。运行时启用态必须统一使用 `User.EffectiveActive(now)`（启用且未过期）。
2. **入站模板与节点覆盖单一来源**：
   - `Node` 仅存连接地址（`ConnectAddr()` 优先 `Domain` 回退 `IP`）与 SSH 参数。
   - `InboundProfile` 存共享协议参数；`NodeProfileKey` 存节点级密钥材料（AES-GCM 加密）与覆盖项（端口、SNI、指纹等）。
   - **任何节点级覆盖字段必须通过统一 helper 读取**（如端口走 `effectiveKeyPort` / `effectivePort`、Reality 参数走 `EffectiveRealitySNI`、入站标签走 `xray.InboundTag`），严禁消费方直接读模板默认值。
3. **特殊协议边界**：
   - **Hysteria2**：非 Xray-core 协议。仅在端口冲突检测（标为 UDP）与订阅输出层支持；`internal/xray/config.go::buildInbound` 故意拒绝生成、`LiveSyncService` 跳过，严禁塞入 Xray `config.json`。
   - **HTTP / SOCKS5 运维代理**：面向受控程序访问，必须配置源 IP 白名单或账号认证之一（拒绝生成公网开放代理）；**严禁进入 `/sub/:token` 任何订阅输出**；`allow_private` 为真时其直连规则必须前置于 `geoip:private` 屏蔽规则。
4. **路由模板、同步事务与安全回滚（v0.7+）**：
   - 路由采用 `RoutingTemplate`（草稿）→ `RoutingTemplateRevision`（不可变修订）→ `RoutingBinding`（`node > group > global` 继承，多分组不一致显式报冲突，存量旧节点支持 `legacy` 基础路由）。
   - 节点状态区分期望态、磁盘态与已确认运行态（`NodeConfigState`）。向节点写配置的所有路径必须获取进程内 `lockNode(nodeID)` 及数据库租约 `AcquireConfigLease`。
   - **回滚安全铁律**：同步后健康探测失败自动回滚 `LastHealthyPolicy` 时，必须结合**当前最新有效用户与密钥**重新渲染，绝不能通过回放旧文件复活已禁用/删除/过期用户的权限；自动恢复不得篡改管理员已保存的期望绑定。
   - **漂移检测**：长期保持 SSH 读取远端 `config.json` 对比 SHA256，不改用 gRPC `StatsService`（idle 用户无统计计数器会导致漏报）。

---

## 3. 关键代码路径速查

```text
config/                         # 启动静态配置结构（DB 种子来源）
pkg/
  crypto/                       # MasterKey AES-GCM 加解密 + SHA256 哈希
  ssh/                          # SSH 连接、gRPC 隧道拨号、远端预检/原子替换事务与真实代理探测
  types/                        # 协议配置结构、传输层分类与覆盖 helper
internal/
  entity/                       # GORM 实体（admin/user/node/group/inbound_profile/routing/alert/traffic）
  repository/                   # 数据访问层、配置租约与级联清理
  xray/                         # Xray 配置渲染、路由策略校验合并、gRPC Handler/Stats 客户端与 vendored proto
  service/                      # 核心编排（sync/livesync/routing/config_*/subscribe*/traffic*/alert/health/install）
  handler/                      # Gin 路由（router.go）、中间件与订阅 HTML 模板
  scheduler/                    # 后台周期任务（健康检测、流量采集与清理、漂移检测、过期用户摘除、备份）
frontend/src/
  index.css                     # 全局唯一主题色变量定义（亮/暗两套语义变量）
  lib/                          # API 封装、协议徽章单一来源（protocol.ts）、路由模型（routing-model.ts）
  components/ui/                # 通用组件（Drawer/Table/Form/ListToolbar/Badge 等）
  components/routing/           # 路由模板、绑定覆盖与配置预览组件
  pages/                        # 页面组件（Dashboard/Nodes/Profiles/Routing/Users/Groups/Logs/settings/*）
docs/design/                    # 纳入版本管理的系统设计方案文档
release-notes/                  # 各版本发布说明（GitHub Action 发版强校验）
```

---

## 4. 历史踩坑清单（Gotchas）

| 领域 | 原因与强制规范 |
|---|---|
| **AES-GCM 随机 Nonce 误标漂移** | 重复保存未修改的加密密钥（如 Reality 私钥、出站凭据）时，必须比对解密明文并复用原密文（或基于解密规范化结构算等价摘要），否则每次加密密文不同会导致无关节点被误标待同步 |
| **变更失效范围最小化** | 修改分组、共享协议模板、单节点密钥/端口时，必须在同一数据库事务中仅对有效配置实际变化的节点调用失效标记，严禁无脑 `MarkAllDrifted()` |
| **远端临时文件后缀** | Xray `26.11.9+` 对无 `.json` 后缀的临时配置文件预检会报 `PREFLIGHT_FAILED`；远端预检、恢复与健康探测生成的临时文件必须保留 `.json` 后缀并显式传 `-format json` |
| **vendored proto 注册约束** | protobuf 全局注册表按文件路径去重，`handlerpb` 顶层文件不能与 `statspb` 重名；且 proto `package`（如 `xray.proxy.vless`）决定 gRPC `AddUser` 的 type URL，必须与 `xray-core` 逐字一致，仅 `go_package` 可改 |
| **告警状态并发回写** | service 按需实例化，互斥锁必须用包级锁；`node_alert_states` 推进用 `SaveObservation`、送达用 `MarkNotified` 做字段级更新，严禁整行 `Save` 覆盖并发期间的恢复状态 |
| **操作日志必须带 Actor** | `LogRepository` 只保留 `RecordWithActor`；HTTP handler 传 `actorFrom(c)`（`admin:<username>`），后台任务传对应 `system:*` 常量 |
| **GORM AutoMigrate 与布尔默认值** | 新增表/列通过 AutoMigrate 自动迁移，新字段必须保证旧库零侵入兼容；对带 `default:true` 的字段插入 `false` 时注意 GORM RETURNING 回填问题 |
| **前端二进制下载鉴权** | 原生 `<a href download>` 不携带 Authorization JWT 头，受保护文件下载必须走 `axios` `blob` + `URL.createObjectURL` |
| **MasterKey 离线备份** | 数据库中的私钥密文依赖 `XRAY_PILOT_MASTER_KEY` 或 `config.yaml` 中的 `master_key`，迁移恢复 SQLite 备份时必须配套保留原 `master_key` |

---

## 5. 开发、前端与 Git 协作规范

### 5.1 构建与测试命令

```bash
# 后端编译、静态检查与全量单测
go build ./...
go vet ./...
go test ./...

# 可选：指定本地 Xray 二进制执行真实内核配置预检兼容测试
# XRAY_TEST_BINARY=/path/to/xray go test ./...

# 前端代码检查、单测与生产构建（发布前必须构建以更新 go:embed 产物）
cd frontend
npm run lint
npm test
npm run build
```

### 5.2 代码与前端样式规范
- **语言与风格**：与用户沟通使用中文，回复保持简短直接；后端注释跟随既有上下文使用中文；**严禁在代码、UI 或文档中使用 emoji**（图标统一用 `lucide-react`）。
- **主题与色彩单源**：所有颜色只通过 `frontend/src/index.css` 的 CSS 变量（`--accent`、`--danger`、`--warning`、`--success`、`--code-bg` 等）表达；**严禁在组件中硬编码 `emerald-*` / `rose-*` / `slate-*` 等 Tailwind 调色板类，严禁使用 `dark:` 变体**。
- **字体与离线可用**：仅使用已通过 `@fontsource/*` 内嵌的 `Inter` 与 `IBM Plex Mono`，严禁引入 Google Fonts 或外部 CDN 资源。
- **布局约定**：`Table` 自带外框，禁止外层再套卡片形成双边框；页面首行直接放 `ListToolbar`；抽屉宽度按 `md(560)` / `lg(720)` / `xl(960)` 规范选用；`Switch` 统一从 `components/ui/Form.tsx` 导入。

### 5.3 决策、文档目录与 Git 提交流程
- **先对齐后编码**：涉及跨模块或架构边界的改动，先与用户对齐关键决策，并将定稿方案沉淀至 `docs/design/`。
- **文档目录与临时文件约定**：
  - `docs/design/` 仅存放跨模块或里程碑级系统设计文档；命名遵循 [docs/design/README.md](./docs/design/README.md)（未定版用 `YYYY-MM-DD-<topic>.md`，定版/发版用 `YYYY-MM-DD-vX.Y-<topic>.md`），新增、重命名或移除文档时必须同步更新 `docs/design/README.md` 且索引列统一使用文件名链接。
  - 同一版本或主题的补充细节直接修订对应主设计文档，禁止另起零碎短文档。
  - 临时草稿、小型补丁说明、一次性排查/分析笔记统一写入 `tmp/`（已在 `.gitignore` 忽略），严禁写入 `docs/design/`。
- **诚实交付**：遇到底层内核或环境限制不支持的能力（如 Hysteria2 进 Xray、无 IPv6 路由强开 IPv6），主动明确边界并拦截，不伪造表面兼容。
- **不主动执行 Git 写操作**：除非用户明确发出“提交”或“发布”指令，否则不要主动执行 `git commit`、`git push`、`git tag` 或合并 PR。
- **分支与发版约束**：分支统一用 `feature/<短描述>`；提交遵循 `<type>(<scope>): <english subject>`；打 tag 发版前必须先编写面向用户的 `release-notes/vX.Y.Z.md`（CI 强校验）。
