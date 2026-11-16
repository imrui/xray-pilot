import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { AlertTriangle, ArrowRight } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import type { SyncSummary } from '@/types'
import { cn } from '@/lib/utils'

export function SyncStatusPill({ summary, compact }: { summary: SyncSummary; compact?: boolean }) {
  const navigate = useNavigate()
  if (!summary.needs_sync) return null
  return <DropdownMenu.Root>
    <DropdownMenu.Trigger asChild><button type="button" title="有节点待同步" className={cn('inline-flex h-9 items-center gap-1.5 rounded-md border border-[var(--warning-border)] bg-[var(--warning-soft)] text-xs font-semibold text-[var(--warning)]', compact ? 'px-2' : 'px-2.5')}><AlertTriangle className="h-4 w-4" />{compact ? summary.total_affected : `待同步 ${summary.total_affected}`}</button></DropdownMenu.Trigger>
    <DropdownMenu.Portal><DropdownMenu.Content align="end" sideOffset={8} className="z-50 w-80 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3 shadow-[var(--shadow-card)]">
      <p className="text-sm font-semibold">{summary.total_affected} 个节点需要同步</p>
      <p className="my-2 text-xs text-soft">配置变化 {summary.drifted_count} · 同步失败 {summary.failed_count} · 待同步 {summary.pending_count}</p>
      <DropdownMenu.Item onSelect={() => navigate('/nodes?sync_status=needs_sync')} className="flex cursor-pointer items-center justify-end gap-2 rounded-md p-2 text-sm text-[var(--accent)] outline-none hover:bg-[var(--panel-muted)]">前往节点列表处理<ArrowRight className="h-4 w-4" /></DropdownMenu.Item>
    </DropdownMenu.Content></DropdownMenu.Portal>
  </DropdownMenu.Root>
}
