import { type ReactNode, useEffect } from 'react'
import { X } from 'lucide-react'
import { cn } from '@/lib/utils'

interface ModalProps {
  open: boolean
  onClose: () => void
  title: string
  children: ReactNode
  footer?: ReactNode
  size?: 'sm' | 'md' | 'lg'
}

export function Modal({ open, onClose, title, children, footer, size = 'md' }: ModalProps) {
  useEffect(() => {
    document.body.style.overflow = open ? 'hidden' : ''
  }, [open])

  if (!open) return null
  const widths = { sm: 'max-w-sm', md: 'max-w-xl', lg: 'max-w-2xl' }

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center p-4 sm:items-center">
      <div className="absolute inset-0 bg-slate-950/50 backdrop-blur-sm transition-opacity" onClick={onClose} />
      <div className={cn('panel-strong relative mt-10 w-full rounded-[28px] sm:mt-0', widths[size])}>
        <div className="flex items-center justify-between border-b border-[var(--border)] px-6 py-5">
          <h3 className="text-lg font-semibold tracking-[-0.03em]">{title}</h3>
          <button onClick={onClose} className="rounded-xl p-2 text-soft transition-colors hover:bg-[var(--panel-muted)] hover:text-[var(--text)]">
            <X className="h-5 w-5" />
          </button>
        </div>
        <div className="p-6">{children}</div>
        {footer && (
          <div className="flex justify-end gap-3 rounded-b-[28px] border-t border-[var(--border)] bg-[var(--panel-muted)] px-6 py-4">
            {footer}
          </div>
        )}
      </div>
    </div>
  )
}
