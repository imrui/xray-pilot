# 系统设计文档（Design Docs）

本目录用于存放 `xray-pilot` 跨模块特性、核心架构演进的设计方案与边界决策记录，纳入 Git 版本管理。

## 目录职责边界

- `docs/design/`：定稿或评审中的**系统设计方案**（背景、架构取舍、数据结构、安全护栏、分阶段计划），随代码库长期演进保留。
- `release-notes/`：面向最终用户与部署者的**版本发布说明**（功能变更、升级注意事项、验证范围）。

## 命名与状态约定

- 文件命名：`YYYY-MM-DD-<topic>.md`（按立项/里程碑日期前缀排序，便于追溯演进脉络）。
- 文档头部统一标注当前状态：`Proposed`（方案评审） / `Approved`（已对齐待实施） / `In Progress`（分阶段实施中） / `Implemented`（已落地发布）。

## 文档索引（按演进时间序）

| 日期 | 文档 | 状态 | 对应版本 | 核心主题 |
|---|---|---|---|---|
| 2026-06-04 | [2026-06-04-v0.3-multi-protocol-and-onboarding.md](./2026-06-04-v0.3-multi-protocol-and-onboarding.md) | Implemented | v0.3.0 ~ v0.5.5 | 多协议入站共存端口模型、Hysteria2 与运维代理边界、无 Agent 一键接入与换机保留关联 |
| 2026-06-24 | [2026-06-24-v0.4.5-grpc-live-sync-and-user-lifecycle.md](./2026-06-24-v0.4.5-grpc-live-sync-and-user-lifecycle.md) | Implemented | v0.2.1 / v0.4.5 / v0.4.9 | SSH 隧道内 gRPC 控制面、Vendored Proto 约束、磁盘+内存双写无重启热同步与到期自动撤权 |
| 2026-09-10 | [2026-09-10-v0.5-admin-auth-alerts-and-ui-system.md](./2026-09-10-v0.5-admin-auth-alerts-and-ui-system.md) | Implemented | v0.5.0 ~ v0.5.2 | 多管理员两级权限模型、全仓 Actor 审计、飞书节点告警持久化状态机与单源 CSS 变量主题 |
| 2026-11-14 | [2026-11-14-v0.6-traffic-analytics-and-dashboard.md](./2026-11-14-v0.6-traffic-analytics-and-dashboard.md) | Implemented | v0.6.0 ~ v0.6.1 / v0.7.2 | 区间流量时间序列聚合、真实零值与缺采断点证据集模型、响应式仪表盘布局 |
| 2026-11-16 | [2026-11-16-v0.7-routing-templates-and-transactional-sync.md](./2026-11-16-v0.7-routing-templates-and-transactional-sync.md) | Implemented | v0.7.0 ~ v0.7.2 | 版本化路由模板（Schema v1/v2）、三级继承与覆盖、远端预检/真实握手探测与不复活旧权限的安全回滚 |
| 2026-11-17 | [2026-11-17-xray-flexible-config-and-routing-v3.md](./2026-11-17-xray-flexible-config-and-routing-v3.md) | Approved | v0.8.0+（待实施） | 统一 `SchemaVersion: 3` 全三期设计：自定义出站、路由全面放开、表单/JSON 双模式、节点按 Tag 覆盖、草稿沙箱预检、DNS 与负载均衡 |
