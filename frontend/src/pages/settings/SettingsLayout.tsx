import { useEffect, useMemo, useState } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { systemApi } from '@/lib/api'
import { useSettingsDraft } from '@/store/settingsDraft'
import { Btn } from '@/components/ui/Form'
import { PageShell, SurfaceCard } from '@/components/ui/Page'
import { Badge } from '@/components/ui/Badge'
import { SETTINGS_SECTIONS, type SettingsFormContext, type SettingsMap, type SettingsSectionKey } from './context'

// 设置页外壳：分组入口在侧栏二级菜单（Layout），这里只持有共享的 KV 表单与底部保存条。
// 表单类分组（节点与同步 / 订阅与通知 / 数据备份）共用一份 form。
export default function SettingsLayout() {
  const qc = useQueryClient()
  const location = useLocation()
  const setDirty = useSettingsDraft((s) => s.setDirty)
  const setDiscard = useSettingsDraft((s) => s.setDiscard)

  const { data: settings, isLoading } = useQuery({
    queryKey: ['system-settings'],
    queryFn: () => systemApi.getSettings().then((r) => r.data.data!),
  })

  // draft 为 null 表示"未编辑"，此时表单直接展示服务端值；保存/撤销都只需把 draft 置回 null
  const [draft, setDraft] = useState<SettingsMap | null>(null)
  const [saved, setSaved] = useState(false)
  const base = useMemo<SettingsMap>(() => settings ?? {}, [settings])
  const form = draft ?? base
  const setForm: React.Dispatch<React.SetStateAction<SettingsMap>> = (next) =>
    setDraft((prev) => (typeof next === 'function' ? next(prev ?? base) : next))

  const update = useMutation({
    mutationFn: () => systemApi.updateSettings(form),
    onSuccess: (res) => {
      qc.setQueryData(['system-settings'], res.data.data)
      void qc.invalidateQueries({ queryKey: ['system-diagnostics'] })
      void qc.invalidateQueries({ queryKey: ['system-feishu-status'] })
      setDraft(null)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    },
  })

  const dirty = draft !== null && JSON.stringify(draft) !== JSON.stringify(base)

  // 把脏状态与丢弃动作交给侧栏，切换分组前拦截；离开设置页时清零
  useEffect(() => {
    setDirty(dirty)
  }, [dirty, setDirty])
  useEffect(() => {
    setDiscard(() => setDraft(null))
    return () => {
      setDiscard(() => {})
      setDirty(false)
    }
  }, [setDiscard, setDirty])

  const currentKey = (location.pathname.split('/')[2] || 'status') as SettingsSectionKey
  const current = SETTINGS_SECTIONS.find((s) => s.key === currentKey) ?? SETTINGS_SECTIONS[0]

  const ctx: SettingsFormContext = {
    form,
    setForm,
    f: (key) => (e) => setForm((p) => ({ ...p, [key]: e.target.value })),
    set: (key, value) => setForm((p) => ({ ...p, [key]: value })),
  }

  if (isLoading) return <div className="p-6 text-soft">加载中…</div>

  return (
    <PageShell>
      <h1 className="sr-only">系统设置 · {current.label}</h1>
      <div className="space-y-6">
        <Outlet context={ctx} />

        {current.form && (
          <SurfaceCard className="sticky bottom-4 flex flex-wrap items-center justify-between gap-3 p-4">
            <div className="flex items-center gap-3 text-sm text-soft">
              <Badge label={saved ? '已保存' : dirty ? '待保存' : '未修改'} variant={saved ? 'green' : dirty ? 'yellow' : 'gray'} />
              <span>
                {saved ? '最近一次更新已写入系统配置。' : dirty ? '有未保存的改动，切换分组前请先保存。' : '当前没有新的配置改动。'}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <Btn variant="secondary" disabled={!dirty || update.isPending} onClick={() => setDraft(null)}>
                撤销
              </Btn>
              <Btn loading={update.isPending} disabled={!dirty} onClick={() => update.mutate()}>
                保存配置
              </Btn>
            </div>
          </SurfaceCard>
        )}
      </div>
    </PageShell>
  )
}
