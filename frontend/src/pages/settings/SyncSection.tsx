import { Field, SelectField, Switch } from '@/components/ui/Form'
import { HintTip } from '@/components/ui/HintTip'
import { Section } from './shared'
import { getXrayStableVersionError, useSettingsForm } from './context'

const LOG_LEVEL_OPTIONS = [
  { value: 'warning', label: 'warning' },
  { value: 'info', label: 'info' },
  { value: 'debug', label: 'debug' },
  { value: 'error', label: 'error' },
]

// 节点与同步：SSH 默认参数、面板出网 IP、Xray 安装版本与运行参数、调度周期
export default function SyncSection() {
  const { form, f, set } = useSettingsForm()
  const liveApplyEnabled = (form['xray.live_apply_enabled'] ?? 'true') === 'true'
  const stableVersionError = getXrayStableVersionError(form['xray.stable_version'])
  const keyPath = form['ssh.default_key_path'] ?? ''
  const driftInterval = form['scheduler.drift_check_interval'] ?? '300'

  return (
    <>
      <Section title="SSH 默认参数" description="节点未指定连接参数时使用这些默认值。面板服务账号需能读取 SSH 私钥。">
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="SSH 端口" type="number" value={form['ssh.default_port'] ?? '22'} onChange={f('ssh.default_port')} />
          <Field label="SSH 用户" value={form['ssh.default_user'] ?? 'root'} onChange={f('ssh.default_user')} />
          <div>
            <Field label="私钥路径" value={keyPath} onChange={f('ssh.default_key_path')} placeholder="/etc/xray-pilot/ssh/id_ed25519" />
            {keyPath.trim().startsWith('/root/.ssh/') && (
              <p className="mt-2 text-sm text-[var(--warning)]">使用 systemd 部署时，建议将私钥放在服务账号可读取的 /etc/xray-pilot/ssh/ 目录。</p>
            )}
          </div>
          <Field label="known_hosts 路径" value={form['ssh.known_hosts_path'] ?? ''} onChange={f('ssh.known_hosts_path')} placeholder="/var/lib/xray-pilot/known_hosts" />
        </div>
      </Section>

      <Section
        title="面板出网 IP"
        description="一键接入时提示节点防火墙放行此 IP。每小时自动探测一次，多出口或 NAT 环境可手动指定。"
        actions={<HintTip content="默认通过 https://api.ipify.org 探测。设置环境变量 XRAY_PILOT_OUTBOUND_PROBE_URL 可使用私有探针。" />}
      >
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="探测到的 IP（只读）" value={form['panel.outbound_ip_auto'] ?? ''} placeholder="启动后约 10 秒首次探测" readOnly />
          <Field label="手动指定 IP" value={form['panel.outbound_ip_manual'] ?? ''} onChange={f('panel.outbound_ip_manual')} placeholder="留空使用探测值" />
        </div>
      </Section>

      <Section title="Xray 安装与探测" description="新生成的接入和更换命令使用所选正式版本，现有节点需单独升级。同步后通过节点代理访问公开 HTTPS 地址验证连接。">
        <div className="grid gap-4 md:grid-cols-2">
          <Field
            label="Xray 稳定版本号"
            value={form['xray.stable_version'] ?? '26.3.27'}
            onChange={f('xray.stable_version')}
            placeholder="26.3.27"
            error={stableVersionError}
            aria-invalid={!!stableVersionError}
            autoComplete="off"
            spellCheck={false}
          />
          <Field label="健康探测 URL" value={form['xray.health_probe_url'] ?? ''} onChange={f('xray.health_probe_url')} placeholder="https://www.gstatic.com/generate_204" />
        </div>
      </Section>

      <Section title="Xray 运行参数" description="访问日志填 none 可关闭，错误日志留空时输出到 stderr。节点可单独覆盖日志级别。">
        <div className="grid gap-4 md:grid-cols-[2fr_2fr_1fr]">
          <Field label="访问日志路径" value={form['xray.log_access'] ?? ''} onChange={f('xray.log_access')} placeholder="none" />
          <Field label="错误日志路径" value={form['xray.log_error'] ?? ''} onChange={f('xray.log_error')} placeholder="/var/log/xray/error.log" />
          <SelectField label="日志级别" value={form['xray.log_level'] ?? 'warning'} onChange={(v) => set('xray.log_level', v)} options={LOG_LEVEL_OPTIONS} />
        </div>
        <div className="border-t border-[var(--border)] pt-4">
          <label className="flex cursor-pointer items-center justify-between gap-4 text-sm font-medium">
            用户变更即时生效（gRPC）
            <Switch checked={liveApplyEnabled} onChange={(next) => set('xray.live_apply_enabled', next ? 'true' : 'false')} />
          </label>
          <p className="mt-1 text-sm leading-6 text-soft">
            {liveApplyEnabled ? '用户增删与过期下线可直接生效。失败的节点会标记为待同步。' : '用户变更将在下次完整同步时生效。'}
          </p>
        </div>
      </Section>

      <Section title="调度周期" description="重启面板后使用新间隔，填 0 可关闭对应任务。过期用户随漂移检测周期下线。明细保留 0 天表示永久保留，累计流量始终保留。">
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <div>
            <Field label="漂移检测间隔（秒）" type="number" value={driftInterval} onChange={f('scheduler.drift_check_interval')} />
            {driftInterval.trim() !== '' && Number(driftInterval) <= 0 && (
              <p className="mt-2 text-sm text-soft">关闭漂移检测后，面板仍每 5 分钟检查一次过期用户。</p>
            )}
          </div>
          <Field label="健康检测间隔（秒）" type="number" value={form['scheduler.health_check_interval'] ?? '120'} onChange={f('scheduler.health_check_interval')} />
          <Field label="流量采集间隔（秒）" type="number" value={form['scheduler.traffic_poll_interval'] ?? '300'} onChange={f('scheduler.traffic_poll_interval')} />
          <Field label="流量明细保留时间（天）" type="number" value={form['traffic.sample_retention_days'] ?? '90'} onChange={f('traffic.sample_retention_days')} />
        </div>
      </Section>
    </>
  )
}
