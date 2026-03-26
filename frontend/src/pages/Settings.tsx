import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { systemApi } from '@/lib/api'
import { Btn, Field, SelectField } from '@/components/ui/Form'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'

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
        <Field label="默认密钥路径（留空使用系统默认）" value={form['ssh.default_key_path'] ?? ''} onChange={f('ssh.default_key_path')} placeholder="~/.ssh/id_rsa" />
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

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <SurfaceCard className="p-6">
      <div className="mb-5">
        <h3 className="text-base font-semibold tracking-[-0.03em]">{title}</h3>
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
