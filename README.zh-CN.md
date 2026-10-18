# xray-pilot

面向小团队的自托管 Xray 节点管理面板。一行命令接入节点，通过 SSH 下发配置，借助 Xray gRPC API 即时增删用户，并分发多格式订阅；前端内嵌，部署只需一个 Go 二进制。

[English README](./README.md)

## 功能特性

- **多协议节点**：支持 VLESS + Reality、VLESS + WS + TLS、Trojan 入站，以及 Hysteria2 订阅条目；协议模板共享参数，端口、SNI、指纹可按节点覆盖
- **节点接入**：面板生成一行安装命令，新 VPS 执行后自动注册回面板；更换服务器时保留分组、密钥与覆盖配置
- **配置同步**：SSH 原子上传配置，定时 SHA256 漂移检测；用户增删改经 Xray gRPC API 即时生效，无需重启 Xray
- **健康检测与告警**：探测 SSH 与代理端口，节点不可达、Xray 端口不通或同步失败时推送飞书群卡片告警，恢复后自动通知
- **订阅分发**：按客户端自动输出 base64 / Clash / sing-box，浏览器访问显示带二维码与流量的信息页；用户到期后自动从各节点下线
- **用户与权限**：通过分组决定用户可用的节点；订阅链接可经飞书机器人私聊发送
- **运维代理**：HTTP / SOCKS5 入站，支持 IP 白名单与可选认证，不进入用户订阅
- **后台管理**：多管理员账号（超级管理员 / 管理员）、带操作人的操作日志、SQLite 定时备份，运行时配置可在 Web UI 调整
- **单文件部署**：React 前端通过 `go:embed` 打包进二进制；默认 SQLite，可选 PostgreSQL

## 界面截图

> 截图均使用虚构的演示数据。

### 登录页

![登录页](./docs/screenshots/01-login.png)

控制台登录入口，支持亮色与暗色主题。

### 仪表盘

![仪表盘](./docs/screenshots/02-dashboard.png)

集中展示节点健康、活跃用户、累计流量、最近操作和待同步节点。

### 节点管理

![节点管理](./docs/screenshots/03-nodes.png)

按状态、健康度、地区、所有者服务端筛选；展示同步状态、Xray 版本、已绑定协议，并提供一键接入。

### 用户管理

![用户管理](./docs/screenshots/04-users.png)

维护订阅用户、分组与到期时间；复制订阅链接、查看二维码与流量，或通过飞书推送订阅。

### 协议配置

![协议配置](./docs/screenshots/05-profiles.png)

协议模板维护共享参数，节点密钥维护各节点的密钥材料与端口覆盖。

### 节点告警

![节点告警](./docs/screenshots/06-alerts.png)

选择飞书告警群，调整触发 / 恢复阈值与告警类型，并可发送测试消息。

### 系统设置

![系统设置](./docs/screenshots/07-settings.png)

查看运行诊断与部署建议；调度、SSH、订阅、备份、管理员等配置按分组管理。

### 订阅页

![订阅页](./docs/screenshots/08-subscribe.png)

同一个 `/sub/{token}` 入口按访问场景返回浏览器信息页（二维码导入、节点级复制、备用链接）或客户端订阅内容。

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
- 创建 `/etc/xray-pilot/ssh/` 目录用于存放服务读取的 SSH 私钥
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

### 节点初始化

对于新节点，可以使用仓库内的 [`scripts/node-bootstrap.sh`](./scripts/node-bootstrap.sh) 快速完成基础准备。脚本会依次执行：

- 调整 `sshd_config` 中的全局 `PermitRootLogin`
- 可选写入 root 的 `authorized_keys`
- 安装并重启 `xray`
- 可选开启 `BBR`
- 最后尝试重启 SSH 服务

推荐流程：

1. 切换到 `root` 用户执行脚本。
2. 按提示粘贴用于管理节点的 SSH 公钥；直接回车可跳过这一步。
3. 脚本结束后确认输出中的 `Xray version` 和 `TCP congestion control`。

推荐直接执行：

```bash
curl -fsSL https://raw.githubusercontent.com/imrui/xray-pilot/main/scripts/node-bootstrap.sh | sudo bash
```

如果你希望非交互执行，也可以在运行前传入 `AUTHORIZED_KEYS`：

```bash
curl -fsSL https://raw.githubusercontent.com/imrui/xray-pilot/main/scripts/node-bootstrap.sh | \
sudo AUTHORIZED_KEYS=$'ssh-ed25519 AAAA... admin@main\nssh-ed25519 AAAA... root@main' bash
```

如果你更习惯先下载再执行，也可以：

```bash
curl -fsSL https://raw.githubusercontent.com/imrui/xray-pilot/main/scripts/node-bootstrap.sh -o /root/node-bootstrap.sh
chmod +x /root/node-bootstrap.sh
bash /root/node-bootstrap.sh
```

注意事项：

- 如果你跳过公钥输入，脚本不会中断，但会提示你手动检查 `/root/.ssh/authorized_keys`。
- 脚本会尽量只修改 `sshd_config` 全局段里的 `PermitRootLogin`，不会主动改动 `Match` 块。
- 脚本修改前会自动备份 `sshd_config`，备份文件名类似 `/etc/ssh/sshd_config.bak.20260401-221500`。
- 部分发行版的 SSH 服务名可能不同，脚本会自动尝试 `ssh` 和 `sshd`；如果都失败，会输出提醒，需手工重启。
- 建议在云厂商安全组、防火墙和系统防火墙中同时确认 `22` 端口可访问。

#### 自动接入模式（v0.4.0+，推荐）

如果你已经部署了 panel，可以在面板里点「一键接入」生成一行 curl 命令，脚本会自动完成「拉 panel 公钥 → 写 authorized_keys → 装 xray → BBR → 重启 SSH → 回填节点元数据」整套流程，**不再需要手工填公钥、也不用回到面板新增节点**。

命令形如：

```bash
curl -fsSL https://raw.githubusercontent.com/imrui/xray-pilot/main/scripts/node-bootstrap.sh | \
  sudo PANEL_URL=https://your-panel INSTALL_TOKEN=abc123... bash
```

`PANEL_URL` / `INSTALL_TOKEN` 由面板对话框直接复制；脚本检测到这两个变量后自动进入自动模式。token 默认 10 分钟有效、一次性、绑定首次访问 IP；过期或被使用后将无法再次执行。

自动模式与原有交互模式互不影响——若环境变量未设置则按原方式提示输入公钥。

## 升级

已经通过一键安装部署的 Linux 机器，可以直接重新执行安装脚本完成升级：

```bash
curl -fsSL https://raw.githubusercontent.com/imrui/xray-pilot/main/install.sh | sudo bash
```

升级过程会：

- 用最新 Release 二进制替换 `/usr/local/bin/xray-pilot`
- 保留现有 `/etc/xray-pilot/config.yaml`
- 保留现有 `/etc/xray-pilot/ssh/` 密钥目录
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

ssh:
  default_port: 22
  default_user: "root"
  default_key_path: ""
  known_hosts_path: "/var/lib/xray-pilot/known_hosts"

admins:
  - username: admin
    password: "change-me-now"
```

对于 Linux systemd 部署，推荐将 SSH 私钥放在 `/etc/xray-pilot/ssh/id_ed25519`，服务使用的 `known_hosts` 文件保存在 `/var/lib/xray-pilot/known_hosts`。

调度周期、SSH 默认值、订阅格式、Xray 日志等级等运行时配置，已经迁移到数据库中的系统配置表，可在 Web UI 里调整。

v0.5.0 起管理员账号保存在数据库中。`admins` 段只在首次启动（管理员表为空）时写入，第一个账号为超级管理员；之后请在「系统设置 → 管理员」中维护。

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
