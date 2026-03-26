# xray-pilot

面向小团队的自托管 Xray 管理面板。它可以通过 SSH 下发配置、检测配置漂移，并用单个嵌入前端的 Go 二进制提供完整管理界面。

[English README](./README.md)

## 功能特性

- 管理 Xray Reality 节点的创建、编辑、同步与启停
- 定时执行配置漂移检测，基于 SHA256 比较本地与远端配置
- 管理用户、分组与订阅访问权限
- 生成多格式订阅，并根据节点健康状态优先选择可用节点
- 通过 SSH 原子上传配置并重启远端服务
- 使用 `go:embed` 打包前端，部署时只需要一个后端二进制
- 运行时系统配置写入数据库，可直接在 Web UI 中调整

## 技术栈

| 层 | 技术 |
| --- | --- |
| 后端 | Go 1.26.1+、Gin、GORM、Zap |
| 数据库 | 默认 SQLite，可选 PostgreSQL |
| 鉴权 | JWT Bearer Token、bcrypt |
| 加密 | AES-GCM 存储敏感字段，SHA256 做漂移校验 |
| 前端 | React 19、Vite、TypeScript、Tailwind CSS v4 |

## 安装

### 一键安装

Linux `amd64` 和 `arm64` 机器可以直接执行：

```bash
curl -fsSL https://raw.githubusercontent.com/imrui/xray-pilot/main/install.sh | sudo bash
```

安装脚本会自动完成：

- 检测系统架构
- 下载最新 GitHub Release
- 校验 `checksums.txt`
- 将二进制安装到 `/usr/local/bin/xray-pilot`
- 生成 `/etc/xray-pilot/config.yaml`
- 创建并启动 `xray-pilot.service`
- 生成随机管理员密码、`jwt.secret` 和 `crypto.master_key`

### 手动安装

| 平台 | 压缩包 |
| --- | --- |
| Linux amd64 | `xray-pilot_<version>_linux_amd64.tar.gz` |
| Linux arm64 | `xray-pilot_<version>_linux_arm64.tar.gz` |
| macOS amd64 | `xray-pilot_<version>_darwin_amd64.tar.gz` |
| macOS arm64 | `xray-pilot_<version>_darwin_arm64.tar.gz` |

手动安装步骤：

1. 从最新的 [GitHub Release](https://github.com/imrui/xray-pilot/releases) 下载对应平台压缩包和 `checksums.txt`。
2. 使用 `sha256sum -c checksums.txt --ignore-missing` 校验文件。
3. 解压后将二进制放到目标路径。
4. 将 [`config.yaml.example`](./config.yaml.example) 复制为 `config.yaml` 并修改配置。
5. 执行 `./xray-pilot` 启动服务。

## 升级

已经通过一键安装部署的 Linux 机器，可以直接重新执行安装脚本完成升级：

```bash
curl -fsSL https://raw.githubusercontent.com/imrui/xray-pilot/main/install.sh | sudo bash
```

升级过程会：

- 用最新 Release 二进制替换 `/usr/local/bin/xray-pilot`
- 保留现有 `/etc/xray-pilot/config.yaml`
- 保留现有 SQLite 数据库或外部数据库配置
- 重新加载并重启 `xray-pilot.service`

升级后可以通过下面的命令确认当前运行版本：

```bash
journalctl -u xray-pilot -n 20 --no-pager
```

## 从源码启动

前置要求：Go 1.26.1+、Node.js 24+

```bash
git clone https://github.com/imrui/xray-pilot.git
cd xray-pilot

make build

cp config.yaml.example config.yaml
# 生产环境请先修改 jwt.secret 和 crypto.master_key

./xray-pilot
```

启动后访问 `http://localhost:2026`，使用 `config.yaml` 中配置的管理员账号登录。

## 配置说明

将 [`config.yaml.example`](./config.yaml.example) 复制为 `config.yaml`：

```yaml
server:
  port: 2026
  mode: release

database:
  driver: sqlite
  dsn: xray-pilot.db

jwt:
  secret: "change-me-use-a-long-random-string"
  expire: 24

crypto:
  master_key: ""

admins:
  - username: admin
    password: "change-me-now"
```

调度周期、SSH 默认值、订阅格式、Xray 日志等级等运行时配置，已经迁移到数据库中的系统配置表，可在 Web UI 里调整。

也可以通过环境变量覆盖主密钥：

```bash
XRAY_PILOT_MASTER_KEY=<hex-key> ./xray-pilot
```

优先级：

1. `XRAY_PILOT_MASTER_KEY`
2. `config.yaml -> crypto.master_key`
3. 首次启动自动生成

## 开发

```bash
# 后端
make dev-backend

# 前端
make dev-frontend
```

前端开发服务器会将 API 请求代理到 `http://localhost:2026`。

## 自动发布

推送形如 `v*` 的 tag 后，GitHub Actions 会自动：

1. 使用 Node 24 构建前端
2. 使用 Go 1.26 编译发布二进制
3. 打包 Linux 和 macOS 的 `amd64`、`arm64` 压缩包
4. 生成 `checksums.txt`
5. 创建并发布 GitHub Release

## 许可证

MIT，见 [LICENSE](./LICENSE)。
