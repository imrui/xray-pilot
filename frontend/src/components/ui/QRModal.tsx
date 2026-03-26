import { useState } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import { Check, Copy, X } from 'lucide-react'

interface QRModalProps {
  open: boolean
  onClose: () => void
  url: string
  title?: string
}

export function QRModal({ open, onClose, url, title = '订阅二维码' }: QRModalProps) {
  const [copied, setCopied] = useState(false)

  if (!open) return null

  const handleCopy = () => {
    navigator.clipboard.writeText(url)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/50 p-4 backdrop-blur-sm">
      <div className="panel-strong w-full max-w-sm rounded-[28px]">
        <div className="flex items-center justify-between border-b border-[var(--border)] p-6">
          <h3 className="text-lg font-semibold tracking-[-0.03em]">{title}</h3>
          <button onClick={onClose} className="rounded-xl p-2 text-soft transition-colors hover:bg-[var(--panel-muted)] hover:text-[var(--text)]">
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="flex flex-col items-center gap-4 p-6">
          <div className="rounded-[24px] border border-[var(--border)] bg-white p-4">
            <QRCodeSVG value={url} size={200} level="M" />
          </div>

          <div className="w-full">
            <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-faint">订阅链接</p>
            <div className="flex items-center gap-2">
              <input readOnly value={url} className="flex-1 rounded-2xl border bg-[var(--panel-muted)] px-3 py-3 text-xs text-soft" />
              <button
                onClick={handleCopy}
                className="shrink-0 flex items-center gap-1.5 rounded-2xl bg-[var(--accent)] px-4 py-3 text-sm font-semibold text-slate-950 transition hover:brightness-105"
              >
                {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                {copied ? '已复制' : '复制'}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
