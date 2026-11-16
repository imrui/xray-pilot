import { useCallback, useMemo, useState, useRef, type ReactNode } from 'react'
import { AlertTriangle, ShieldAlert } from 'lucide-react'
import { Btn } from '@/components/ui/Form'
import * as Dialog from '@radix-ui/react-dialog'

import { ConfirmContext, type ConfirmOptions, type ConfirmFn } from '@/lib/confirm-context'

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const returnFocus = useRef<HTMLElement | null>(null)
  const [state, setState] = useState<(ConfirmOptions & { open: boolean }) | null>(null)
  const [resolver, setResolver] = useState<((value: boolean) => void) | null>(null)

  const confirm = useCallback<ConfirmFn>((options) => {
    return new Promise<boolean>((resolve) => {
      setState({ open: true, ...options })
      setResolver(() => resolve)
    })
  }, [])

  const close = (value: boolean) => {
    resolver?.(value)
    setResolver(null)
    setState(null)
  }

  const value = useMemo(() => confirm, [confirm])

  return (
    <ConfirmContext.Provider value={value}>
      {children}
      {state?.open && (
        <Dialog.Root open onOpenChange={open => { if (!open) close(false) }}><Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 z-[70] bg-[var(--overlay)] backdrop-blur-sm" />
          <Dialog.Content {...(!state.description ? { 'aria-describedby': undefined } : {})} onOpenAutoFocus={() => { returnFocus.current = document.activeElement as HTMLElement }} onCloseAutoFocus={event => { event.preventDefault(); returnFocus.current?.focus() }} className="panel-strong fixed left-1/2 top-1/2 z-[70] w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 overflow-hidden rounded-[28px] outline-none">
            <div className="border-b border-[var(--border)] px-6 py-5">
              <div className="flex items-start gap-4">
                <div className={`mt-0.5 flex h-11 w-11 items-center justify-center rounded-2xl ${state.tone === 'danger' ? 'bg-[var(--danger-soft)] text-[var(--danger)]' : 'bg-[var(--warning-soft)] text-[var(--warning)]'}`}>
                  {state.tone === 'danger' ? <ShieldAlert className="h-5 w-5" /> : <AlertTriangle className="h-5 w-5" />}
                </div>
                <div>
                  <Dialog.Title className="text-lg font-semibold tracking-[-0.03em]">{state.title}</Dialog.Title>
                  {state.description && <Dialog.Description className="mt-1.5 text-sm leading-6 text-soft">{state.description}</Dialog.Description>}
                </div>
              </div>
            </div>
            <div className="flex justify-end gap-3 bg-[var(--panel-muted)] px-6 py-4">
              <Btn variant="secondary" onClick={() => close(false)}>{state.cancelText ?? '取消'}</Btn>
              <Btn variant={state.tone === 'danger' ? 'danger' : 'primary'} onClick={() => close(true)}>
                {state.confirmText ?? '确认'}
              </Btn>
            </div>
          </Dialog.Content>
        </Dialog.Portal></Dialog.Root>
      )}
    </ConfirmContext.Provider>
  )
}
