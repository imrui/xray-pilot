import { useMutation, useQueryClient } from '@tanstack/react-query'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { AlertTriangle, ArrowRight, RefreshCw } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { nodeApi } from '@/lib/api'
import { pushToast } from '@/lib/notify'
import type { SyncSummary } from '@/types'
import { cn } from '@/lib/utils'

function buildSummaryText(summary: SyncSummary) {
  const parts: string[] = []
  if (summary.drifted_count > 0) parts.push(`配置漂移 ${summary.drifted_count} 个`)
  if (summary.failed_count > 0) parts.push(`同步失败 ${summary.failed_count} 个`)
  if (summary.pending_count > 0) parts.push(`待同步 ${summary.pending_count} 个`)
  return parts.join('，')
}

// 顶栏「待同步」状态胶囊：常驻但不占内容区；点击展开明细与快捷操作。
// 待同步是持续状态而非一次性事件，因此不用会消失的 toast，也不用挤内容的横幅。
export function SyncStatusPill({ summary, compact }: { summary: SyncSummary; compact?: boolean }) {
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
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          title="有节点待同步"
          className={cn(
            'inline-flex h-9 items-center gap-1.5 rounded-md border border-[var(--warning-border)] bg-[var(--warning-soft)] text-xs font-semibold text-[var(--warning)] transition hover:brightness-110',
            compact ? 'px-2' : 'px-2.5'
          )}
        >
          <AlertTriangle className="h-4 w-4" />
          {compact ? summary.total_affected : `待同步 ${summary.total_affected}`}
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={8}
          className="z-50 w-80 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3 shadow-[var(--shadow-card)]"
        >
          <div className="flex items-start gap-3">
            <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-[var(--warning-soft)] text-[var(--warning)]">
              <AlertTriangle className="h-4 w-4" />
            </div>
            <div className="min-w-0 space-y-1">
              <div className="text-sm font-semibold">检测到配置变更，部分节点需要重新同步</div>
              <p className="text-xs leading-5 text-soft">
                待处理节点 {summary.total_affected} 个。
                {buildSummaryText(summary) && <span> 当前包含 {buildSummaryText(summary)}。</span>}
              </p>
            </div>
          </div>
          <DropdownMenu.Separator className="my-3 h-px bg-[var(--border)]" />
          <div className="flex items-center justify-end gap-2">
            <DropdownMenu.Item
              onSelect={() => navigate('/nodes')}
              className="inline-flex h-9 cursor-pointer select-none items-center gap-1 rounded-md px-3 text-sm text-soft outline-none transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
            >
              前往节点管理
              <ArrowRight className="h-4 w-4" />
            </DropdownMenu.Item>
            <DropdownMenu.Item
              disabled={syncMutation.isPending}
              onSelect={() => syncMutation.mutate()}
              className="inline-flex h-9 cursor-pointer select-none items-center gap-1.5 rounded-md border border-[var(--warning-border)] bg-[var(--warning-soft)] px-3 text-sm font-medium text-[var(--warning)] outline-none transition hover:brightness-110 data-[disabled]:opacity-50"
            >
              <RefreshCw className={cn('h-4 w-4', syncMutation.isPending && 'animate-spin')} />
              一键同步
            </DropdownMenu.Item>
          </div>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
