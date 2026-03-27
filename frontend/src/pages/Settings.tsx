import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { systemApi } from '@/lib/api'
import { Btn, Field, SelectField } from '@/components/ui/Form'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'
import { Badge } from '@/components/ui/Badge'
import type { DiagnosticItem } from '@/types'
import { CheckCircle2, CircleAlert, ShieldCheck, Wrench } from 'lucide-react'

const LOG_LEVEL_OPTIONS = [
  { value: 'warning', label: 'warning' },
  { value: 'info', label: 'info' },
  { value: 'debug', label: 'debug' },
  { value: 'error', label: 'error' },
]

type SettingsMap = Record<string, string>

function toForm(s: SettingsMap): SettingsMap {
  return { ...s }
}

export default function Settings() {
  const qc = useQueryClient()

  const { data: info } = useQuery({
    queryKey: ['system-info'],
    queryFn: () => systemApi.getInfo().then((r) => r.data.data!),
  })

  const { data: settings, isLoading } = useQuery({
    queryKey: ['system-settings'],
    queryFn: () => systemApi.getSettings().then((r) => r.data.data!),
  })

  const { data: diagnostics, refetch: refetchDiagnostics, isFetching: diagnosticsLoading } = useQuery({
    queryKey: ['system-diagnostics'],
    queryFn: () => systemApi.getDiagnostics().then((r) => r.data.data!),
  })

  const [form, setForm] = useState<SettingsMap>({})
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (settings) setForm(toForm(settings))
  }, [settings])

  const f = (key: string) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((p) => ({ ...p, [key]: e.target.value }))

  const update = useMutation({
    mutationFn: () => systemApi.updateSettings(form),
    onSuccess: (res) => {
      qc.setQueryData(['system-settings'], res.data.data)
      void qc.invalidateQueries({ queryKey: ['system-diagnostics'] })
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    },
  })

  if (isLoading) return <div className="p-6 text-soft">加载中…</div>

  return (
    <PageShell className="max-w-5xl">
      <PageHeader
        title="系统设置"
        description="管理服务端运行参数、日志级别和默认连接策略，保存后即时更新当前配置。"
        stats={[
          { label: '服务端口', value: info?.server.port ?? '—' },
          { label: '运行模式', value: info?.server.mode ?? '—' },
          { label: '数据库', value: info?.database.driver ?? '—' },
        ]}
      />

      <SurfaceCard className="grid gap-4 p-4 md:grid-cols-3">
        <ReadOnlyItem label="服务端口" value={String(info?.server.port ?? '—')} />
        <ReadOnlyItem label="运行模式" value={String(info?.server.mode ?? '—')} />
        <ReadOnlyItem label="数据库" value={String(info?.database.driver ?? '—')} />
      </SurfaceCard>

      <Section
        title="部署诊断"
        actions={
          <Btn variant="secondary" loading={diagnosticsLoading} onClick={() => void refetchDiagnostics()}>
            刷新诊断
          </Btn>
        }
      >
        <div className="grid gap-3 md:grid-cols-3">
          <ReadOnlyItem label="正常项" value={String(diagnostics?.summary.ok ?? 0)} />
          <ReadOnlyItem label="警告项" value={String(diagnostics?.summary.warning ?? 0)} />
          <ReadOnlyItem label="错误项" value={String(diagnostics?.summary.error ?? 0)} />
        </div>
        <div className="grid gap-3">
          {diagnostics?.items.map((item) => <DiagnosticCard key={item.key} item={item} />)}
        </div>
        <div className="rounded-[22px] border border-[var(--border)] bg-[var(--panel-muted)] p-4 text-sm leading-6 text-soft">
          <div className="flex items-center gap-2 font-semibold text-[var(--text)]">
            <Wrench className="h-4 w-4" />
            部署建议
          </div>
          <p className="mt-2">systemd 部署建议使用 `/etc/xray-pilot/ssh/id_ed25519` 作为默认私钥路径，并将 `subscription.base_url` 显式配置为你的公网 HTTPS 域名。</p>
        </div>
      </Section>

      <Section title="Xray 日志">
        <Field label="访问日志路径（none 表示关闭）" value={form['xray.log_access'] ?? ''} onChange={f('xray.log_access')} placeholder="none" />
        <Field label="错误日志路径（留空使用 stderr）" value={form['xray.log_error'] ?? ''} onChange={f('xray.log_error')} placeholder="/var/log/xray/error.log" />
        <SelectField
          label="日志级别"
          value={form['xray.log_level'] ?? 'warning'}
          onChange={(v) => setForm((p) => ({ ...p, 'xray.log_level': v }))}
          options={LOG_LEVEL_OPTIONS}
        />
      </Section>

      <Section title="订阅配置">
        <Field
          label="订阅链接前缀（留空则自动从请求 Host 获取）"
          value={form['subscription.base_url'] ?? ''}
          onChange={f('subscription.base_url')}
          placeholder="https://your-domain.com"
        />
        <p className="text-xs text-soft">如果服务部署在 Nginx、CDN 或 HTTPS 反向代理后，建议显式填写公网访问地址，避免订阅链接依赖代理头推断。</p>
        <div>
          <Field
            label="节点备注格式"
            value={form['subscription.remark_format'] ?? ''}
            onChange={f('subscription.remark_format')}
            placeholder="{node_name} ({username}) [{protocol} - {transport}]"
          />
          <p className="mt-2 text-xs text-soft">可用占位符：{'{node_name}'} {'{username}'} {'{protocol}'} {'{transport}'} {'{region}'}</p>
        </div>
      </Section>

      <Section title="SSH 默认参数">
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="默认端口" type="number" value={form['ssh.default_port'] ?? '22'} onChange={f('ssh.default_port')} />
          <Field label="默认用户" value={form['ssh.default_user'] ?? 'root'} onChange={f('ssh.default_user')} />
        </div>
        <Field label="默认密钥路径（推荐系统服务专用路径）" value={form['ssh.default_key_path'] ?? ''} onChange={f('ssh.default_key_path')} placeholder="/etc/xray-pilot/ssh/id_ed25519" />
        <Field label="known_hosts 路径" value={form['ssh.known_hosts_path'] ?? ''} onChange={f('ssh.known_hosts_path')} placeholder="/var/lib/xray-pilot/known_hosts" />
        <p className="text-xs text-soft">建议不要直接使用 `/root/.ssh/*`。对于 systemd 部署，请把服务可读的 SSH 私钥放在 `/etc/xray-pilot/ssh/` 下。</p>
      </Section>

      <Section title="定时任务">
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="漂移检测间隔（秒，0 禁用）" type="number" value={form['scheduler.drift_check_interval'] ?? '300'} onChange={f('scheduler.drift_check_interval')} />
          <Field label="健康检测间隔（秒，0 禁用）" type="number" value={form['scheduler.health_check_interval'] ?? '120'} onChange={f('scheduler.health_check_interval')} />
        </div>
        <p className="text-xs text-soft">注意：修改定时间隔需重启服务生效。</p>
      </Section>

      <div className="flex items-center gap-3 pt-1">
        <Btn loading={update.isPending} onClick={() => update.mutate()}>保存所有配置</Btn>
        {saved && <span className="text-sm text-emerald-500">已保存</span>}
      </div>
    </PageShell>
  )
}

function Section({ title, children, actions }: { title: string; children: React.ReactNode; actions?: React.ReactNode }) {
  return (
    <SurfaceCard className="p-6">
      <div className="mb-5 flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <h3 className="text-base font-semibold tracking-[-0.03em]">{title}</h3>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </div>
      <div className="space-y-4">{children}</div>
    </SurfaceCard>
  )
}

function ReadOnlyItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="panel-muted rounded-[22px] p-4">
      <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-faint">{label}</div>
      <div className="mt-2 font-mono text-sm">{value}</div>
    </div>
  )
}

function DiagnosticCard({ item }: { item: DiagnosticItem }) {
  const icon = item.status === 'ok'
    ? <CheckCircle2 className="h-4 w-4 text-emerald-600" />
    : item.status === 'warning'
      ? <CircleAlert className="h-4 w-4 text-amber-600" />
      : <ShieldCheck className="h-4 w-4 text-rose-600" />

  const badgeVariant = item.status === 'ok' ? 'green' : item.status === 'warning' ? 'yellow' : 'red'
  const badgeLabel = item.status === 'ok' ? '正常' : item.status === 'warning' ? '警告' : '错误'

  return (
    <div className="rounded-[22px] border border-[var(--border)] bg-[var(--panel-muted)] p-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            {icon}
            <div className="text-sm font-semibold">{item.label}</div>
          </div>
          {item.value && <div className="font-mono text-xs text-soft">{item.value}</div>}
          <p className="text-sm leading-6 text-soft">{item.detail}</p>
          {item.suggestion && <p className="text-xs leading-5 text-faint">建议：{item.suggestion}</p>}
        </div>
        <Badge label={badgeLabel} variant={badgeVariant} />
      </div>
    </div>
  )
}
