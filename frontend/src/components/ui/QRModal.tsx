import { useEffect, useId, useRef, useState } from 'react'
import { QRCodeCanvas } from 'qrcode.react'
import { Check, Copy, Download } from 'lucide-react'
import { copyText } from '@/lib/clipboard'
import { pushToast } from '@/lib/notify'
import { Btn } from './Form'
import { Modal } from './Modal'

interface QRModalProps {
  onClose: () => void
  url: string
  username: string
  relay: boolean
}

const primaryFormats = ['', 'v2ray', 'clash', 'singbox'] as const
const relayFormats = ['', 'json', 'clash'] as const
const formatLabels = { '': '订阅', v2ray: 'V2Ray', clash: 'Clash', singbox: 'sing-box', json: 'JSON' }

export function QRModal({ onClose, url, username, relay }: QRModalProps) {
  const title = `${username} 的${relay ? '内网中转订阅' : '订阅'}二维码`
  const [format, setFormat] = useState<keyof typeof formatLabels>('')
  const [copiedURL, setCopiedURL] = useState('')
  const radioName = useId()
  const canvas = useRef<HTMLCanvasElement>(null)
  const formats = relay ? relayFormats : primaryFormats
  const selectedURL = format ? `${url}?format=${format}` : url
  const copied = selectedURL === copiedURL

  useEffect(() => {
    if (!copiedURL) return
    const timer = setTimeout(() => setCopiedURL(''), 2000)
    return () => clearTimeout(timer)
  }, [copiedURL])

  function saveQRCode() {
    if (!canvas.current) return
    const link = document.createElement('a')
    link.href = canvas.current.toDataURL('image/png')
    const fileUser = Array.from(username.trim().replace(/[^\p{L}\p{M}\p{N}._@+-]+/gu, '_')).slice(0, 48).join('') || 'user'
    link.download = `xray-pilot-${fileUser}-${relay ? 'intranet-relay' : 'subscription'}-${format || 'default'}.png`
    document.body.appendChild(link)
    link.click()
    link.remove()
  }

  return (
    <Modal open onClose={onClose} title={title} size="sm">
      <div className="flex flex-col items-center gap-4">
        <fieldset className="flex max-w-full flex-wrap justify-center rounded-md border border-[var(--border)] bg-[var(--panel-muted)] p-1">
          <legend className="sr-only">{relay ? '内网中转订阅格式' : '订阅格式'}</legend>
          {formats.map(option => (
            <label key={option} className="cursor-pointer">
              <input className="peer sr-only" type="radio" name={radioName} value={option}
                checked={format === option} onChange={() => setFormat(option)} />
              <span className="block rounded px-2 py-1.5 text-sm text-soft peer-checked:bg-[var(--panel-strong)] peer-checked:font-semibold peer-checked:text-[var(--accent)] peer-focus-visible:outline sm:px-3">
                {formatLabels[option]}
              </span>
            </label>
          ))}
        </fieldset>
        <QRCodeCanvas ref={canvas} value={selectedURL} size={512} marginSize={4} level="M" role="img" aria-label={title}
          style={{ width: 220, height: 'auto', maxWidth: '100%' }}
          className="rounded-xl border border-[var(--border)]" />
        <code className="max-w-full select-all break-all text-center font-mono text-xs leading-relaxed text-soft">{selectedURL}</code>
        <div className="flex flex-wrap justify-center gap-2">
          <Btn type="button" variant="secondary" className="h-9 rounded-full px-3" onClick={async () => {
            if (await copyText(selectedURL)) setCopiedURL(selectedURL)
            else pushToast({ title: '复制失败，请选中上方地址手动复制', variant: 'warning' })
          }}>
            {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
            {copied ? '已复制' : '复制地址'}
          </Btn>
          <Btn type="button" variant="secondary" className="h-9 rounded-full px-3" onClick={saveQRCode}>
            <Download className="h-4 w-4" />保存二维码
          </Btn>
        </div>
      </div>
    </Modal>
  )
}
