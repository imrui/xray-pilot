import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { systemApi } from '@/lib/api'
import { useState, useEffect } from 'react'
import { Btn, Field, SelectField } from '@/components/ui/Form'

const LOG_LEVEL_OPTIONS = [
  { value: 'warning', label: 'warning' },
  { value: 'info',    label: 'info' },
  { value: 'debug',   label: 'debug' },
  { value: 'error',   label: 'error' },
]

type SettingsMap = Record<string, string>

// 将 SettingsMap 转换为表单状态
function toForm(s: SettingsMap): SettingsMap {
  return { ...s }
}

export default function Settings() {
  const qc = useQueryClient()

  const { data: info } = useQuery({
    queryKey: ['system-info'],
    queryFn: () => systemApi.getInfo().then(r => r.data.data!),
  })

  const { data: settings, isLoading } = useQuery({
    queryKey: ['system-settings'],
    queryFn: () => systemApi.getSettings().then(r => r.data.data!),
  })

  const [form, setForm] = useState<SettingsMap>({})
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (settings) setForm(toForm(settings))
  }, [settings])

  const f = (key: string) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm(p => ({ ...p, [key]: e.target.value }))

  const update = useMutation({
    mutationFn: () => systemApi.updateSettings(form),
    onSuccess: (res) => {
      qc.setQueryData(['system-settings'], res.data.data)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    },
  })

  if (isLoading) return <div className="p-6 text-slate-400">加载中…</div>

  return (
    <div className="p-6 space-y-6 max-w-2xl">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">系统设置</h1>
        <p className="text-sm text-slate-500 mt-0.5">管理运行时配置，保存后立即生效（定时任务间隔重启后生效）</p>
      </div>

      {/* 只读系统信息 */}
      <div className="bg-slate-50 rounded-xl border border-slate-200 p-4 space-y-2 text-sm">
        <p className="text-xs font-semibold text-slate-500 uppercase tracking-wide mb-2">系统信息（只读）</p>
        <div className="flex justify-between">
          <span className="text-slate-500">服务端口</span>
          <span className="font-mono text-slate-900">{info?.server.port}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-slate-500">运行模式</span>
          <span className="font-mono text-slate-900">{info?.server.mode}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-slate-500">数据库</span>
          <span className="font-mono text-slate-900">{info?.database.driver}</span>
        </div>
      </div>

      {/* Xray 日志 */}
      <Section title="Xray 日志">
        <Field
          label="访问日志路径（none 表示关闭）"
          value={form['xray.log_access'] ?? ''}
          onChange={f('xray.log_access')}
          placeholder="none"
        />
        <Field
          label="错误日志路径（留空使用 stderr）"
          value={form['xray.log_error'] ?? ''}
          onChange={f('xray.log_error')}
          placeholder="/var/log/xray/error.log"
        />
        <SelectField
          label="日志级别"
          value={form['xray.log_level'] ?? 'warning'}
          onChange={v => setForm(p => ({ ...p, 'xray.log_level': v }))}
          options={LOG_LEVEL_OPTIONS}
        />
      </Section>

      {/* 订阅配置 */}
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
          <p className="mt-1 text-xs text-slate-400">
            可用占位符：{'{node_name}'} {'{username}'} {'{protocol}'} {'{transport}'} {'{region}'}
          </p>
        </div>
      </Section>

      {/* SSH 默认参数 */}
      <Section title="SSH 默认参数">
        <div className="grid grid-cols-2 gap-3">
          <Field
            label="默认端口"
            type="number"
            value={form['ssh.default_port'] ?? '22'}
            onChange={f('ssh.default_port')}
          />
          <Field
            label="默认用户"
            value={form['ssh.default_user'] ?? 'root'}
            onChange={f('ssh.default_user')}
          />
        </div>
        <Field
          label="默认密钥路径（留空使用系统默认）"
          value={form['ssh.default_key_path'] ?? ''}
          onChange={f('ssh.default_key_path')}
          placeholder="~/.ssh/id_rsa"
        />
      </Section>

      {/* 定时任务 */}
      <Section title="定时任务">
        <div className="grid grid-cols-2 gap-3">
          <Field
            label="漂移检测间隔（秒，0 禁用）"
            type="number"
            value={form['scheduler.drift_check_interval'] ?? '300'}
            onChange={f('scheduler.drift_check_interval')}
          />
          <Field
            label="健康检测间隔（秒，0 禁用）"
            type="number"
            value={form['scheduler.health_check_interval'] ?? '120'}
            onChange={f('scheduler.health_check_interval')}
          />
        </div>
        <p className="text-xs text-slate-400">注意：修改定时间隔需重启服务生效</p>
      </Section>

      <div className="flex items-center gap-3 pt-2">
        <Btn loading={update.isPending} onClick={() => update.mutate()}>保存所有配置</Btn>
        {saved && <span className="text-sm text-green-600">已保存</span>}
      </div>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-white rounded-xl border border-slate-200 shadow-sm p-4 space-y-3">
      <h3 className="text-sm font-semibold text-slate-900">{title}</h3>
      {children}
    </div>
  )
}
