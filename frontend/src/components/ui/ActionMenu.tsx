import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { MoreHorizontal } from 'lucide-react'
import { cn } from '@/lib/utils'

interface ActionItem {
  label: string
  onSelect: () => void
  danger?: boolean
  disabled?: boolean
}

export function ActionMenu({ items, label = '更多操作' }: { items: ActionItem[]; label?: string }) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          aria-label={label}
          className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-[var(--border)] bg-[var(--panel-strong)] text-soft transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
        >
          <MoreHorizontal className="h-4 w-4" />
        </button>
      </DropdownMenu.Trigger>

      <DropdownMenu.Portal>
        <DropdownMenu.Content
          sideOffset={8}
          align="end"
          className="z-50 min-w-[176px] rounded-2xl border border-[var(--border)] bg-[var(--panel-strong)] p-1.5 shadow-[var(--shadow-card)]"
        >
          {items.map((item) => (
            <DropdownMenu.Item
              key={item.label}
              disabled={item.disabled}
              onSelect={item.onSelect}
              className={cn(
                'flex cursor-pointer select-none items-center rounded-xl px-3 py-2 text-sm outline-none transition',
                item.danger ? 'text-[var(--danger)] hover:bg-[var(--danger-soft)]' : 'text-[var(--text)] hover:bg-[var(--panel-muted)]',
                item.disabled && 'pointer-events-none opacity-40'
              )}
            >
              {item.label}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
