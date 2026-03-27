import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, CircleAlert, Database, ServerCog, ShieldCheck, Wrench } from 'lucide-react'
import { systemApi } from '@/lib/api'
import { Btn, Field, SelectField } from '@/components/ui/Form'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'
import { Badge } from '@/components/ui/Badge'
import type { DiagnosticItem } from '@/types'

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
    <PageShell>
      <PageHeader
        title="系统设置"
        description="将运行参数、诊断信息和部署建议放在同一页处理，减少部署后期在文档和日志之间来回切换。"
        actions={
          <Btn loading={update.isPending} onClick={() => update.mutate()}>
            保存所有配置
          </Btn>
        }
        stats={[
          { label: '服务端口', value: info?.server.port ?? '—' },
          { label: '运行模式', value: info?.server.mode ?? '—' },
          { label: '数据库', value: info?.database.driver ?? '—' },
        ]}
      />

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="space-y-6">
          <SurfaceCard className="grid gap-4 p-4 md:grid-cols-3">
            <ReadOnlyItem icon={<ServerCog className="h-4 w-4 text-[var(--accent)]" />} label="服务端口" value={String(info?.server.port ?? '—')} />
            <ReadOnlyItem icon={<ShieldCheck className="h-4 w-4 text-[var(--accent)]" />} label="运行模式" value={String(info?.server.mode ?? '—')} />
            <ReadOnlyItem icon={<Database className="h-4 w-4 text-[var(--accent)]" />} label="数据库" value={String(info?.database.driver ?? '—')} />
          </SurfaceCard>

          <Section
            title="部署诊断"
            description="直接在界面里确认服务路径、数据库目录、SSH 默认路径和订阅基址配置。"
            actions={
              <Btn variant="secondary" loading={diagnosticsLoading} onClick={() => void refetchDiagnostics()}>
                刷新诊断
              </Btn>
            }
          >
            <div className="grid gap-3 md:grid-cols-3">
              <ReadOnlyMini label="正常项" value={String(diagnostics?.summary.ok ?? 0)} />
              <ReadOnlyMini label="警告项" value={String(diagnostics?.summary.warning ?? 0)} />
              <ReadOnlyMini label="错误项" value={String(diagnostics?.summary.error ?? 0)} />
            </div>
            <div className="grid gap-3">
              {diagnostics?.items.map((item) => <DiagnosticCard key={item.key} item={item} />)}
            </div>
          </Section>

          <Section title="Xray 日志" description="控制访问日志、错误日志和日志级别。">
            <Field label="访问日志路径（none 表示关闭）" value={form['xray.log_access'] ?? ''} onChange={f('xray.log_access')} placeholder="none" />
            <Field label="错误日志路径（留空使用 stderr）" value={form['xray.log_error'] ?? ''} onChange={f('xray.log_error')} placeholder="/var/log/xray/error.log" />
            <SelectField
              label="日志级别"
              value={form['xray.log_level'] ?? 'warning'}
              onChange={(v) => setForm((p) => ({ ...p, 'xray.log_level': v }))}
              options={LOG_LEVEL_OPTIONS}
            />
          </Section>

          <Section title="订阅配置" description="优先明确公网访问地址和备注模板，避免订阅链接依赖反代环境推断。">
            <Field
              label="订阅链接前缀（留空则自动从请求 Host 获取）"
              value={form['subscription.base_url'] ?? ''}
              onChange={f('subscription.base_url')}
              placeholder="https://your-domain.com"
            />
            <p className="text-xs text-soft">如果服务部署在 Nginx、CDN 或 HTTPS 反向代理后，建议显式填写公网访问地址。</p>
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

          <Section title="SSH 默认参数" description="这里的默认值会影响节点 SSH 测试、同步和 known_hosts 初始化。">
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="默认端口" type="number" value={form['ssh.default_port'] ?? '22'} onChange={f('ssh.default_port')} />
              <Field label="默认用户" value={form['ssh.default_user'] ?? 'root'} onChange={f('ssh.default_user')} />
            </div>
            <Field label="默认密钥路径（推荐系统服务专用路径）" value={form['ssh.default_key_path'] ?? ''} onChange={f('ssh.default_key_path')} placeholder="/etc/xray-pilot/ssh/id_ed25519" />
            <Field label="known_hosts 路径" value={form['ssh.known_hosts_path'] ?? ''} onChange={f('ssh.known_hosts_path')} placeholder="/var/lib/xray-pilot/known_hosts" />
            <p className="text-xs text-soft">建议不要直接使用 `/root/.ssh/*`。对于 systemd 部署，请把服务可读的 SSH 私钥放在 `/etc/xray-pilot/ssh/` 下。</p>
          </Section>

          <Section title="定时任务" description="修改后通常需要重启服务以确保新周期生效。">
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="漂移检测间隔（秒，0 禁用）" type="number" value={form['scheduler.drift_check_interval'] ?? '300'} onChange={f('scheduler.drift_check_interval')} />
              <Field label="健康检测间隔（秒，0 禁用）" type="number" value={form['scheduler.health_check_interval'] ?? '120'} onChange={f('scheduler.health_check_interval')} />
            </div>
          </Section>
        </div>

        <div className="space-y-6">
          <SurfaceCard className="p-5">
            <div className="flex items-center gap-2 text-sm font-semibold">
              <Wrench className="h-4 w-4 text-[var(--accent)]" />
              部署建议
            </div>
            <div className="mt-4 space-y-3 text-sm leading-6 text-soft">
              <p>systemd 部署建议使用 `/etc/xray-pilot/ssh/id_ed25519` 作为默认私钥路径，并将 `subscription.base_url` 显式配置为公网 HTTPS 域名。</p>
              <p>`known_hosts` 建议保持在 `/var/lib/xray-pilot/known_hosts`，方便与运行用户权限对齐。</p>
              <p>如果配置页保存后没有达到预期，先看上方诊断项，再结合日志页排查。</p>
            </div>
          </SurfaceCard>

          <SurfaceCard className="p-5">
            <div className="text-sm font-semibold">保存状态</div>
            <div className="mt-3 flex items-center gap-2">
              <Badge label={saved ? '已保存' : '待保存'} variant={saved ? 'green' : 'gray'} />
              <span className="min-w-0 text-sm text-soft">{saved ? '最近一次更新已写入系统配置。' : '修改完成后点击顶部保存按钮提交。'}</span>
            </div>
          </SurfaceCard>
        </div>
      </div>
    </PageShell>
  )
}

function Section({
  title,
  description,
  children,
  actions,
}: {
  title: string
  description?: string
  children: React.ReactNode
  actions?: React.ReactNode
}) {
  return (
    <SurfaceCard className="p-6">
      <div className="mb-5 flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <h3 className="text-base font-semibold tracking-[-0.03em]">{title}</h3>
          {description && <p className="mt-2 text-sm leading-6 text-soft">{description}</p>}
        </div>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </div>
      <div className="space-y-4">{children}</div>
    </SurfaceCard>
  )
}

function ReadOnlyItem({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return (
    <div className="rounded-[20px] border border-[var(--border)] bg-[var(--panel-muted)] p-4">
      <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">
        {icon}
        {label}
      </div>
      <div className="mt-2 font-mono text-sm">{value}</div>
    </div>
  )
}

function ReadOnlyMini({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-[18px] border border-[var(--border)] bg-[var(--panel-muted)] p-4">
      <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">{label}</div>
      <div className="mt-2 text-lg font-semibold">{value}</div>
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
    <div className="rounded-[20px] border border-[var(--border)] bg-[var(--panel-muted)] p-4">
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
