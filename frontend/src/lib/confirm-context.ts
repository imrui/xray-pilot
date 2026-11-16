import { createContext, useContext } from 'react'

export interface ConfirmOptions {
  title: string
  description?: string
  confirmText?: string
  cancelText?: string
  tone?: 'default' | 'danger'
}
export type ConfirmFn = (options: ConfirmOptions) => Promise<boolean>
export const ConfirmContext = createContext<ConfirmFn | null>(null)
export function useConfirm() {
  const context = useContext(ConfirmContext)
  if (!context) throw new Error('useConfirm must be used within ConfirmProvider')
  return context
}
