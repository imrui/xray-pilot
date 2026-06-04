# Release Notes

Create one Markdown file per tag in this directory.

Naming convention:

- `release-notes/v0.1.0.md`
- `release-notes/v0.1.1.md`

When a tag matching `v*` is pushed, the release workflow will read `release-notes/<tag>.md` and use that file as the GitHub Release body.

If the file is missing, the workflow will fail so the release is not published with an empty description.

## 风格指南

Release notes 直接发布到 GitHub Release 页面，面向**最终用户与运维者**。控制总字数（一般 < 300 字），按功能条目组织，避免开发者视角的实现细节。

历史所有版本（v0.1.0 ~ v0.3.1）已按此风格回填并同步覆盖到 GitHub Release 页面。

### 推荐结构

```markdown
# xray-pilot vX.Y.Z

一句话主题。

## 新增
- 用户视角的功能描述（"你现在能做 X"，而不是"我们实现了 X"）

## 修复
- 简短描述（仅在有用户可见 bug 修复时出现）

## 升级
- DB 迁移行为 / API 兼容性 / 升级方式

## 注意事项
- 安全提醒、使用前置条件（可选，仅在确有需要时出现）
```

### 应该写的

- 每个功能 1-3 句，说清"是什么、用户怎么用"
- 数据库 schema 变化（自动迁移 / 需手动操作）
- 破坏性变更（如有）
- 安全相关的使用提醒（如：含敏感凭据的操作流程）

### 不应该写的

- 实现要点（文件路径、helper 函数名、单测列表）
- 内部架构演进（为未来 vX.Y 预留的字段等）
- 工程权衡（哪些推迟到下个版本、原因）
- commit hash / PR 链接（GitHub release 页面会自动显示）

这些信息属于 commit message 和 PR 描述，不是 release notes 的职责。
