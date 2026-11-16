import { type ReactNode, useRef } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { cn } from '@/lib/utils'

interface ModalProps {
  open: boolean
  onClose: () => void
  title: string
  children: ReactNode
  footer?: ReactNode
  size?: 'sm' | 'md' | 'lg' | 'xl'
}

export function Modal({ open, onClose, title, children, footer, size = 'md' }: ModalProps) {
  const returnFocus = useRef<HTMLElement | null>(null)
  if (!open) return null
  const widths = { sm: 'max-w-sm', md: 'max-w-xl', lg: 'max-w-2xl', xl: 'max-w-6xl' }

  return (
    <Dialog.Root open={open} onOpenChange={next => { if (!next) onClose() }}><Dialog.Portal>
      <Dialog.Overlay className="fixed inset-0 z-[60] bg-[var(--overlay)] backdrop-blur-sm transition-opacity" />
      <Dialog.Content aria-describedby={undefined} onOpenAutoFocus={() => { returnFocus.current = document.activeElement as HTMLElement }} onCloseAutoFocus={event => { event.preventDefault(); returnFocus.current?.focus() }} className={cn('panel-strong fixed left-1/2 top-1/2 z-[60] flex max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 flex-col rounded-[28px] outline-none', widths[size])}>
        <div className="flex items-center justify-between border-b border-[var(--border)] px-6 py-5">
          <Dialog.Title className="text-lg font-semibold tracking-[-0.03em]">{title}</Dialog.Title>
          <button aria-label="关闭对话框" onClick={onClose} className="rounded-xl p-2 text-soft transition-colors hover:bg-[var(--panel-muted)] hover:text-[var(--text)]">
            <X className="h-5 w-5" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-6">{children}</div>
        {footer && (
          <div className="flex justify-end gap-3 rounded-b-[28px] border-t border-[var(--border)] bg-[var(--panel-muted)] px-6 py-4">
            {footer}
          </div>
        )}
      </Dialog.Content>
    </Dialog.Portal></Dialog.Root>
  )
}
