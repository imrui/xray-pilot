import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import { AlertTriangle, ShieldAlert } from 'lucide-react'
import { Btn } from '@/components/ui/Form'

type ConfirmTone = 'default' | 'danger'

interface ConfirmOptions {
  title: string
  description?: string
  confirmText?: string
  cancelText?: string
  tone?: ConfirmTone
}

type ConfirmFn = (options: ConfirmOptions) => Promise<boolean>

const ConfirmContext = createContext<ConfirmFn | null>(null)

export function ConfirmProvider({ children }: { children: ReactNode }) {
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
        <div className="fixed inset-0 z-[70] flex items-center justify-center p-4">
          <div className="absolute inset-0 bg-slate-950/50 backdrop-blur-sm" onClick={() => close(false)} />
          <div className="panel-strong relative w-full max-w-md rounded-[28px]">
            <div className="border-b border-[var(--border)] px-6 py-5">
              <div className="flex items-start gap-4">
                <div className={`mt-0.5 flex h-11 w-11 items-center justify-center rounded-2xl ${state.tone === 'danger' ? 'bg-rose-500/12 text-rose-500' : 'bg-amber-500/12 text-amber-500'}`}>
                  {state.tone === 'danger' ? <ShieldAlert className="h-5 w-5" /> : <AlertTriangle className="h-5 w-5" />}
                </div>
                <div>
                  <h3 className="text-lg font-semibold tracking-[-0.03em]">{state.title}</h3>
                  {state.description && <p className="mt-1.5 text-sm leading-6 text-soft">{state.description}</p>}
                </div>
              </div>
            </div>
            <div className="flex justify-end gap-3 bg-[var(--panel-muted)] px-6 py-4">
              <Btn variant="secondary" onClick={() => close(false)}>{state.cancelText ?? '取消'}</Btn>
              <Btn variant={state.tone === 'danger' ? 'danger' : 'primary'} onClick={() => close(true)}>
                {state.confirmText ?? '确认'}
              </Btn>
            </div>
          </div>
        </div>
      )}
    </ConfirmContext.Provider>
  )
}

export function useConfirm() {
  const context = useContext(ConfirmContext)
  if (!context) {
    throw new Error('useConfirm must be used within ConfirmProvider')
  }
  return context
}
