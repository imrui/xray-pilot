import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, ArrowRight, RefreshCw } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { nodeApi } from '@/lib/api'
import { pushToast } from '@/lib/notify'
import type { SyncSummary } from '@/types'
import { Btn } from '@/components/ui/Form'

function buildSummaryText(summary: SyncSummary) {
  const parts: string[] = []
  if (summary.drifted_count > 0) parts.push(`配置漂移 ${summary.drifted_count} 个`)
  if (summary.failed_count > 0) parts.push(`同步失败 ${summary.failed_count} 个`)
  if (summary.pending_count > 0) parts.push(`待同步 ${summary.pending_count} 个`)
  return parts.join('，')
}

export function SyncReminderBanner({ summary }: { summary: SyncSummary }) {
  const navigate = useNavigate()
  const qc = useQueryClient()

  const syncMutation = useMutation({
    mutationFn: () => nodeApi.syncDrifted(),
    onSuccess: (res) => {
      const payload = res.data.data
      pushToast({
        title: '待处理节点已开始同步',
        description: payload
          ? `总计 ${payload.total} 个节点，成功 ${payload.success} 个，失败 ${payload.failed} 个。`
          : '节点同步已完成。',
        variant: payload?.failed ? 'warning' : 'success',
      })
      qc.invalidateQueries({ queryKey: ['sync-summary'] })
      qc.invalidateQueries({ queryKey: ['nodes'] })
      qc.invalidateQueries({ queryKey: ['recent-logs'] })
      qc.invalidateQueries({ queryKey: ['nodes-stats'] })
    },
  })

  if (!summary.needs_sync) return null

  return (
    <section className="mb-4 rounded-lg border border-amber-500/30 bg-[var(--panel-strong)] p-4 shadow-[var(--shadow-card)] md:mb-5">
      <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-amber-500/12 text-amber-500">
            <AlertTriangle className="h-5 w-5" />
          </div>
          <div className="space-y-1">
            <h3 className="text-sm font-semibold tracking-[-0.02em] md:text-base">检测到配置变更，部分节点需要重新同步</h3>
            <p className="text-sm leading-6 text-soft">
              待处理节点 {summary.total_affected} 个。
              {buildSummaryText(summary) && <span> 当前包含 {buildSummaryText(summary)}。</span>}
            </p>
          </div>
        </div>

        <div className="flex flex-wrap gap-2">
          <Btn
            variant="secondary"
            loading={syncMutation.isPending}
            onClick={() => syncMutation.mutate()}
            className="border-amber-500/35 text-amber-500 hover:bg-amber-500/10"
          >
            <RefreshCw className="h-4 w-4" />
            一键同步
          </Btn>
          <Btn variant="ghost" onClick={() => navigate('/nodes')} className="text-soft hover:text-[var(--text)]">
            前往节点管理
            <ArrowRight className="h-4 w-4" />
          </Btn>
        </div>
      </div>
    </section>
  )
}
