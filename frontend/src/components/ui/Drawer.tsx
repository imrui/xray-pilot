import { type ReactNode, useEffect, useRef } from 'react'
import { X } from 'lucide-react'
import * as Dialog from '@radix-ui/react-dialog'
import { cn } from '@/lib/utils'

interface DrawerProps {
  open: boolean
  onClose: () => void
  title: string
  description?: ReactNode
  children: ReactNode
  footer?: ReactNode
  width?: 'md' | 'lg' | 'xl'
  dirty?: boolean
  saving?: boolean
  onBeforeClose?: () => Promise<boolean> | boolean
}

export function Drawer({ open, onClose, title, description, children, footer, width = 'md', dirty = false, saving = false, onBeforeClose }: DrawerProps) {
  const returnFocus = useRef<HTMLElement | null>(null)
  useEffect(() => {
    if (!open || !dirty) return

    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }

    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [open, dirty])

  if (!open) return null

  const widths = {
    md: 'max-w-[560px]',
    lg: 'max-w-[720px]',
    xl: 'max-w-[960px]',
  }

  const requestClose = async () => {
    if (saving) return
    if (onBeforeClose) {
      const allowed = await onBeforeClose()
      if (!allowed) return
    }
    onClose()
  }

  return (
    <Dialog.Root open={open} onOpenChange={next => { if (!next) void requestClose() }}>
      <Dialog.Portal>
      <Dialog.Overlay className="fixed inset-0 z-50 bg-[var(--overlay)] backdrop-blur-sm" />
      <Dialog.Content {...(!description ? { 'aria-describedby': undefined } : {})} onOpenAutoFocus={() => { returnFocus.current = document.activeElement as HTMLElement }} onCloseAutoFocus={event => { event.preventDefault(); returnFocus.current?.focus() }} className={cn('fixed right-0 top-0 z-50 flex h-[100dvh] max-h-[100dvh] w-full min-h-0 flex-col border-l border-[var(--border)] bg-[var(--panel-strong)] shadow-[var(--shadow-panel)] outline-none', widths[width])}>
        <div className="flex items-start justify-between gap-4 border-b border-[var(--border)] px-6 py-5">
          <div>
            <div className="flex items-center gap-2">
              <Dialog.Title className="text-lg font-semibold tracking-[-0.03em]">{title}</Dialog.Title>
              {dirty && <span className="inline-flex h-2.5 w-2.5 rounded-full bg-[var(--warning)]" title="存在未保存更改" />}
            </div>
            {description && <Dialog.Description className="mt-1.5 text-sm leading-6 text-soft">{description}</Dialog.Description>}
          </div>
          <button
            aria-label="关闭抽屉"
            disabled={saving}
            onClick={() => void requestClose()}
            className="rounded-md border border-[var(--border)] bg-[var(--panel)] p-2 text-soft transition hover:text-[var(--text)]"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-auto px-6 py-5">{children}</div>

        {footer && (
          <div className="flex items-center justify-between gap-3 border-t border-[var(--border)] bg-[var(--panel-muted)] px-6 py-4 pb-[calc(1rem+env(safe-area-inset-bottom))]">
            <div className="text-xs text-soft">
              {saving ? '保存中…' : dirty ? '有未保存更改' : '配置以保存结果为准'}
            </div>
            <div className="flex flex-wrap items-center justify-end gap-2">{footer}</div>
          </div>
        )}
      </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
