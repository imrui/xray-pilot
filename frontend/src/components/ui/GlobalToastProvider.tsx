import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Toast } from '@/components/ui/Toast'
import { subscribeToast, type GlobalToastPayload } from '@/lib/notify'

export function GlobalToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<GlobalToastPayload | null>(null)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    return subscribeToast((payload) => {
      setToast(payload)
    })
  }, [])

  useEffect(() => {
    if (!toast) return
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => setToast(null), 3200)
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [toast])

  return (
    <>
      {children}
      {toast && (
        <div className="pointer-events-none fixed right-5 top-5 z-[80]">
          <div className="pointer-events-auto">
            <Toast {...toast} onClose={() => setToast(null)} />
          </div>
        </div>
      )}
    </>
  )
}
