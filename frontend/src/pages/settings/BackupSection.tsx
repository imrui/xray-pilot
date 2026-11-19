import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, Trash2 } from 'lucide-react'
import { backupApi } from '@/lib/api'
import type { BackupFile } from '@/lib/api'
import { Btn, Field } from '@/components/ui/Form'
import { useConfirm } from '@/lib/confirm-context'
import { pushToast } from '@/lib/notify'
import { formatBytes } from '@/lib/utils'
import { Section } from './shared'
import { useSettingsForm } from './context'

// 数据备份：周期 / 目录 / 保留（走统一保存）+ 立即备份与快照文件列表（即时操作）
export default function BackupSection() {
  const { form, f } = useSettingsForm()
  const qc = useQueryClient()
  const confirm = useConfirm()

  const { data, isLoading, isFetching, isError, refetch } = useQuery({
    queryKey: ['system-backups'],
    queryFn: () => backupApi.list().then((r) => r.data.data ?? []),
  })

  const run = useMutation({
    mutationFn: () => backupApi.run(),
    onSuccess: (res) => {
      pushToast({
        title: '备份完成',
        description: `${res.data.data?.name ?? ''} · ${formatBytes(res.data.data?.size)}`,
        variant: 'success',
      })
      void qc.invalidateQueries({ queryKey: ['system-backups'] })
    },
    onError: (e: Error) => {
      pushToast({ title: '备份失败', description: e.message, variant: 'warning' })
    },
  })

  const remove = useMutation({
    mutationFn: (name: string) => backupApi.remove(name),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['system-backups'] }),
  })

  const backups = data ?? []

  return (
    <>
      <Section title="备份策略" description="SQLite 快照请另存到异地。恢复时须使用原 MasterKey，才能解密私钥和证书。">
        <div className="grid gap-4 md:grid-cols-3">
          <Field label="自动备份间隔（小时，0 禁用）" type="number" value={form['backup.interval_hours'] ?? '24'} onChange={f('backup.interval_hours')} />
          <Field label="备份目录（相对工作目录或绝对路径）" value={form['backup.dir'] ?? 'data/backup'} onChange={f('backup.dir')} placeholder="data/backup" />
          <Field label="快照保留天数（0 关闭自动清理）" type="number" value={form['backup.retention_days'] ?? '30'} onChange={f('backup.retention_days')} />
        </div>
      </Section>

      <Section
        title={backups.length ? `备份文件（${backups.length}）` : '备份文件'}
        actions={
          <>
            <Btn variant="ghost" loading={isFetching} onClick={() => void refetch()}>刷新</Btn>
            <Btn variant="secondary" loading={run.isPending} onClick={() => run.mutate()}>立即备份</Btn>
          </>
        }
      >
        <div className="border-t border-[var(--border)]">
          {isError && <p role="alert" className="py-3 text-sm text-[var(--danger)]">读取备份列表失败，请刷新重试。</p>}
          {isLoading ? (
            <div className="py-6 text-sm text-soft">加载中…</div>
          ) : backups.length > 0 ? (
            <ul className="divide-y divide-[var(--border)]">
              {backups.map((b: BackupFile) => (
                <li key={b.name} className="flex flex-wrap items-center justify-between gap-3 py-3 text-sm">
                  <div className="min-w-0">
                    <div className="truncate font-mono text-[13px]">{b.name}</div>
                    <div className="mt-0.5 text-xs text-soft">
                      {formatBytes(b.size)} · {new Date(b.created_at).toLocaleString()}
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Btn
                      variant="ghost"
                      onClick={() => {
                        backupApi.download(b.name).catch((e: Error) => {
                          pushToast({ title: '下载备份失败', description: e.message, variant: 'error' })
                        })
                      }}
                    >
                      <Download className="h-3.5 w-3.5" />
                      下载
                    </Btn>
                    <Btn
                      variant="ghost"
                      onClick={async () => {
                        const ok = await confirm({
                          title: `删除备份 ${b.name}？`,
                          description: '文件将从备份目录永久删除，无法恢复。',
                          confirmText: '删除',
                          cancelText: '取消',
                          tone: 'danger',
                        })
                        if (ok) remove.mutate(b.name)
                      }}
                    >
                      <Trash2 className="h-3.5 w-3.5 text-[var(--danger)]" />
                      删除
                    </Btn>
                  </div>
                </li>
              ))}
            </ul>
          ) : !isError && <div className="py-6 text-sm text-soft">暂无备份文件</div>}
        </div>
      </Section>
    </>
  )
}
