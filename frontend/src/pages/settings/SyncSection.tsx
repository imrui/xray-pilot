import { Field, SelectField, Switch } from '@/components/ui/Form'
import { Section } from './shared'
import { useSettingsForm } from './context'

const LOG_LEVEL_OPTIONS = [
  { value: 'warning', label: 'warning' },
  { value: 'info', label: 'info' },
  { value: 'debug', label: 'debug' },
  { value: 'error', label: 'error' },
]

// 节点与同步：SSH 默认参数、面板出网 IP、Xray 运行参数、调度周期
export default function SyncSection() {
  const { form, f, set } = useSettingsForm()
  const liveApplyEnabled = (form['xray.live_apply_enabled'] ?? 'true') === 'true'

  return (
    <>
      <Section title="SSH 默认参数" description="节点未单独配置时使用的连接参数，影响 SSH 测试、同步和 known_hosts 初始化。">
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="默认端口" type="number" value={form['ssh.default_port'] ?? '22'} onChange={f('ssh.default_port')} />
          <Field label="默认用户" value={form['ssh.default_user'] ?? 'root'} onChange={f('ssh.default_user')} />
        </div>
        <Field label="默认密钥路径（推荐系统服务专用路径）" value={form['ssh.default_key_path'] ?? ''} onChange={f('ssh.default_key_path')} placeholder="/etc/xray-pilot/ssh/id_ed25519" />
        <Field label="known_hosts 路径" value={form['ssh.known_hosts_path'] ?? ''} onChange={f('ssh.known_hosts_path')} placeholder="/var/lib/xray-pilot/known_hosts" />
        <p className="text-xs text-soft">不建议直接使用 `/root/.ssh/*`。systemd 部署请把服务可读的私钥放在 `/etc/xray-pilot/ssh/` 下。</p>
      </Section>

      <Section title="面板出网 IP" description="一键接入时提示用户在节点防火墙放行的 IP。后台每小时自动探测，手动覆盖优先。">
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="自动探测值（只读）" value={form['panel.outbound_ip_auto'] ?? ''} onChange={() => {}} placeholder="启动后约 10 秒填入" readOnly />
          <Field label="手动覆盖（留空则使用自动值）" value={form['panel.outbound_ip_manual'] ?? ''} onChange={f('panel.outbound_ip_manual')} placeholder="多 IP / NAT / 容器场景手动填入" />
        </div>
        <p className="text-xs text-soft">
          通过 `https://api.ipify.org` 探测，可用 `XRAY_PILOT_OUTBOUND_PROBE_URL` 环境变量换成私有探针。容器或反代环境下自动值可能是入口 IP，请改用手动覆盖。
        </p>
      </Section>

      <Section title="Xray 运行参数" description="日志输出与用户变更的生效方式，对所有节点生效；节点可单独覆盖日志级别。">
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="访问日志路径（none 表示关闭）" value={form['xray.log_access'] ?? ''} onChange={f('xray.log_access')} placeholder="none" />
          <Field label="错误日志路径（留空使用 stderr）" value={form['xray.log_error'] ?? ''} onChange={f('xray.log_error')} placeholder="/var/log/xray/error.log" />
        </div>
        <SelectField label="日志级别" value={form['xray.log_level'] ?? 'warning'} onChange={(v) => set('xray.log_level', v)} options={LOG_LEVEL_OPTIONS} />
        <div className="flex items-start justify-between gap-4 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 py-2.5">
          <div>
            <p className="text-sm font-medium text-[var(--text)]">gRPC 即时生效（用户增删不重启）</p>
            <p className="mt-1 text-xs text-soft">
              开启后，用户增删改与过期下线走 xray gRPC 即时生效，不重启 xray、不断开其他用户连接；失败自动回退到「标记漂移 + 重启同步」。
            </p>
          </div>
          <Switch checked={liveApplyEnabled} onChange={(next) => set('xray.live_apply_enabled', next ? 'true' : 'false')} />
        </div>
      </Section>

      <Section title="调度周期" description="后台任务执行间隔。修改后需重启服务才能按新周期运行。">
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="漂移检测间隔（秒，0 禁用）" type="number" value={form['scheduler.drift_check_interval'] ?? '300'} onChange={f('scheduler.drift_check_interval')} />
          <Field label="健康检测间隔（秒，0 禁用）" type="number" value={form['scheduler.health_check_interval'] ?? '120'} onChange={f('scheduler.health_check_interval')} />
          <Field label="流量采集间隔（秒，0 禁用）" type="number" value={form['scheduler.traffic_poll_interval'] ?? '300'} onChange={f('scheduler.traffic_poll_interval')} />
          <Field label="流量明细保留天数（0 不清理，累计不受影响）" type="number" value={form['traffic.sample_retention_days'] ?? '90'} onChange={f('traffic.sample_retention_days')} />
        </div>
        <p className="text-xs text-soft">过期用户自动下线复用漂移检测周期；漂移检测禁用时固定每 5 分钟执行一次。</p>
      </Section>
    </>
  )
}
