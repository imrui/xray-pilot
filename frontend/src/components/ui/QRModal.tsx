import { QRCodeSVG } from 'qrcode.react'
import { X, Copy, Check } from 'lucide-react'
import { useState } from 'react'

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
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-sm">
        {/* 头部 */}
        <div className="flex items-center justify-between p-6 border-b border-gray-200">
          <h3 className="text-lg font-semibold text-gray-900">{title}</h3>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* 二维码 */}
        <div className="p-6 flex flex-col items-center gap-4">
          <div className="p-3 border border-gray-200 rounded-lg">
            <QRCodeSVG value={url} size={200} level="M" />
          </div>

          {/* 订阅链接 */}
          <div className="w-full">
            <p className="text-xs text-gray-500 mb-1.5">订阅链接</p>
            <div className="flex gap-2 items-center">
              <input
                readOnly
                value={url}
                className="flex-1 px-3 py-2 border border-gray-200 rounded-lg text-xs text-gray-600 bg-gray-50 truncate"
              />
              <button
                onClick={handleCopy}
                className="shrink-0 flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm font-medium bg-indigo-600 text-white hover:bg-indigo-700 transition-colors"
              >
                {copied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                {copied ? '已复制' : '复制'}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
