import { useQuery, useMutation } from '@tanstack/react-query'
import { systemApi } from '@/lib/api'
import { useState, useEffect } from 'react'
import { Btn, Field } from '@/components/ui/Form'

export default function Settings() {
  const { data, isLoading } = useQuery({
    queryKey: ['system-config'],
    queryFn: () => systemApi.getConfig().then(r => r.data.data!),
  })

  const [drift, setDrift] = useState('')
  const [health, setHealth] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (data) {
      setDrift(String(data.scheduler.drift_check_interval))
      setHealth(String(data.scheduler.health_check_interval))
    }
  }, [data])

  const update = useMutation({
    mutationFn: () => systemApi.updateConfig({
      drift_check_interval: Number(drift) || 0,
      health_check_interval: Number(health) || 0,
    }),
    onSuccess: () => { setSaved(true); setTimeout(() => setSaved(false), 2000) },
  })

  if (isLoading) return <div className="p-6 text-slate-400">加载中…</div>

  return (
    <div className="p-6 space-y-6 max-w-xl">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">系统设置</h1>
        <p className="text-sm text-slate-500 mt-0.5">查看系统配置，调整定时任务间隔</p>
      </div>

      {/* 只读信息 */}
      <div className="bg-slate-50 rounded-xl border border-slate-200 p-4 space-y-2 text-sm">
        <div className="flex justify-between">
          <span className="text-slate-500">服务端口</span>
          <span className="font-mono text-slate-900">{data?.server.port}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-slate-500">运行模式</span>
          <span className="font-mono text-slate-900">{data?.server.mode}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-slate-500">数据库</span>
          <span className="font-mono text-slate-900">{data?.database.driver}</span>
        </div>
      </div>

      {/* 可调整配置 */}
      <div className="bg-white rounded-xl border border-slate-200 shadow-sm p-4 space-y-4">
        <h3 className="text-sm font-semibold text-slate-900">定时任务</h3>
        <Field
          label="漂移检测间隔（秒，0 禁用）"
          type="number"
          value={drift}
          onChange={e => setDrift(e.target.value)}
        />
        <Field
          label="健康检测间隔（秒，0 禁用）"
          type="number"
          value={health}
          onChange={e => setHealth(e.target.value)}
        />
        <p className="text-xs text-slate-400">注意：修改后重启服务生效</p>
        <div className="flex items-center gap-3">
          <Btn loading={update.isPending} onClick={() => update.mutate()}>保存配置</Btn>
          {saved && <span className="text-sm text-green-600">已保存</span>}
        </div>
      </div>
    </div>
  )
}
