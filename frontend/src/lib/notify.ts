export type GlobalToastPayload = {
  title: string
  description?: string
  variant: 'success' | 'error' | 'warning'
}

type Listener = (payload: GlobalToastPayload) => void

const listeners = new Set<Listener>()

export function subscribeToast(listener: Listener) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function pushToast(payload: GlobalToastPayload) {
  listeners.forEach((listener) => listener(payload))
}
