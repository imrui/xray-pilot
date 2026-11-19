import { type ReactNode, useId, useState } from 'react'
import { ChevronRight, Code2, KeyRound, Link2, Plus, ShieldCheck, SlidersHorizontal, Sparkles, Trash2 } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Btn, Field, SelectField } from '@/components/ui/Form'
import { JsonEditor } from '@/components/ui/JsonEditor'
import { OUTBOUND_SNIPPETS } from '@/lib/routing-snippets'
import { editorItemKey, retainEditorItemKey, wireGuardReservedError } from '@/lib/routing-editor'
import {
  BUILTIN_OUTBOUNDS,
  STRUCTURED_OUTBOUND_PROTOCOLS,
  countOutboundReferences,
  createDefaultOutbound,
  extractDialerProxy,
  isEncryptedSecret,
  isStructuredOutboundProtocol,
  parseSingleObject,
  patchDialerProxy,
  validateOutboundTag,
} from '@/lib/routing-model'
import type { BalancerItem, CustomOutbound, RoutingRuleItem } from '@/lib/routing-model'

const ALL_PROTOCOL_OPTIONS = [
  ...STRUCTURED_OUTBOUND_PROTOCOLS,
  { value: 'trojan', label: 'Trojan（JSON 模式）' },
  { value: 'shadowsocks', label: 'Shadowsocks（JSON 模式）' },
  { value: 'vmess', label: 'VMess（JSON 模式）' },
  { value: 'blackhole', label: 'Blackhole（JSON 模式）' },
]

function SecretInput({
  label,
  value,
  placeholder,
  disabled,
  onChange,
}: {
  label: string
  value: string
  placeholder?: string
  disabled?: boolean
  onChange: (next: string) => void
}) {
  const encrypted = isEncryptedSecret(value)
  const masked = value === '<masked>'
  const [editingCipher, setEditingCipher] = useState(false)

  if (encrypted && !editingCipher) {
    return (
      <div className="flex flex-col space-y-1.5">
        <span className="text-sm font-medium text-soft">{label}</span>
        <div className="flex h-10 items-center justify-between gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] px-3 text-sm">
          <span className="inline-flex items-center gap-1.5 text-soft">
            <KeyRound className="h-3.5 w-3.5 text-[var(--accent)]" />
            已加密保存
          </span>
          {!disabled && (
            <button
              type="button"
              onClick={() => {
                setEditingCipher(true)
                onChange('')
              }}
              className="font-medium text-[var(--accent)] hover:underline"
            >
              重新输入
            </button>
          )}
        </div>
      </div>
    )
  }

  return (
    <Field
      label={masked ? `${label}（当前账号已脱敏只读）` : label}
      type={masked ? 'text' : 'password'}
      autoComplete="new-password"
      value={value}
      disabled={disabled || masked}
      placeholder={placeholder}
      onChange={(e) => onChange(e.target.value)}
    />
  )
}

export function SingleOutboundFields({
  outbound,
  action,
  lockedTag,
  peerOutboundTags = [],
  disabled = false,
  onChange,
  onRawErrorChange,
}: {
  outbound: CustomOutbound
  action?: ReactNode
  lockedTag?: string
  peerOutboundTags?: string[]
  disabled?: boolean
  onChange: (next: CustomOutbound) => void
  onRawErrorChange?: (err: string) => void
}) {
  const structuredSupported = isStructuredOutboundProtocol(outbound.protocol)
  const [mode, setMode] = useState<'form' | 'json'>(structuredSupported ? 'form' : 'json')
  const [rawDraft, setRawDraft] = useState<{ text: string; source: string } | null>(null)
  const serialized = JSON.stringify(outbound)
  const rawText = rawDraft?.source === serialized ? rawDraft.text : JSON.stringify(outbound, null, 2)
  const [rawError, setRawError] = useState('')
  const [reservedDraft, setReservedDraft] = useState<string | null>(null)
  const [reservedError, setReservedError] = useState('')

  const activeMode = !structuredSupported ? 'json' : mode
  const tagError = lockedTag ? '' : validateOutboundTag(outbound.tag)

  const switchMode = (target: 'form' | 'json') => {
    if (target === activeMode || reservedError) return
    if (target === 'json') {
      setRawDraft(null)
      setRawError('')
      onRawErrorChange?.('')
      setMode('json')
      return
    }
    try {
      const parsed = parseSingleObject<CustomOutbound>(rawText, '自定义出站')
      const fixed: CustomOutbound = {
        ...parsed,
        tag: lockedTag ?? (typeof parsed.tag === 'string' ? parsed.tag : outbound.tag),
        protocol: typeof parsed.protocol === 'string' ? parsed.protocol : outbound.protocol,
      }
      if (fixed.protocol === 'wireguard') {
        const error = wireGuardReservedError(fixed.settings?.reserved)
        if (error) throw new Error(error)
      }
      if (!isStructuredOutboundProtocol(fixed.protocol)) {
        setRawError(`协议 ${fixed.protocol} 暂不支持表单模式，请继续使用 JSON 编辑或切换为受支持协议。`)
        return
      }
      onChange(fixed)
      setReservedDraft(null)
      setRawError('')
      onRawErrorChange?.('')
      setMode('form')
    } catch (e) {
      const msg = (e as Error).message
      setRawError(msg)
      onRawErrorChange?.(msg)
    }
  }

  const dialerOptions = [
    { value: '', label: '节点直接拨号' },
    { value: 'direct', label: 'direct（默认直连）' },
    { value: 'direct-v4', label: 'direct-v4（强制 IPv4 拨号）' },
    { value: 'direct-v6', label: 'direct-v6（强制 IPv6 拨号）' },
    ...peerOutboundTags
      .filter((t) => t && t !== outbound.tag)
      .map((t) => ({ value: t, label: `${t}（链式出站）` })),
  ]
  const dialerField = (
    <SelectField
      label="前置出站（可选）"
      value={extractDialerProxy(outbound)}
      onChange={(dp) => onChange(patchDialerProxy(outbound, dp))}
      options={dialerOptions}
    />
  )

  const settings = (outbound.settings && typeof outbound.settings === 'object' ? outbound.settings : {}) as Record<
    string,
    unknown
  >

  const updateProtocol = (nextProto: string) => {
    const template = createDefaultOutbound(nextProto, 1)
    const next: CustomOutbound = {
      ...template,
      tag: lockedTag ?? outbound.tag,
      remark: outbound.remark,
    }
    onChange(next)
    if (!isStructuredOutboundProtocol(nextProto)) {
      setRawDraft(null)
      setMode('json')
    }
    setReservedDraft(null)
    setReservedError('')
    onRawErrorChange?.('')
  }

  // SOCKS / HTTP helpers
  const renderProxyServerFields = () => {
    const servers = Array.isArray(settings.servers) ? [...(settings.servers as Record<string, unknown>[])] : [{}]
    const first = { ...(servers[0] ?? {}) }
    const users = Array.isArray(first.users) ? [...(first.users as Record<string, unknown>[])] : []
    const firstUser = { ...(users[0] ?? {}) }

    const patchServer = (patch: Record<string, unknown>) => {
      const nextFirst = { ...first, ...patch }
      const nextServers = [nextFirst, ...servers.slice(1)]
      onChange({
        ...outbound,
        settings: { ...settings, servers: nextServers },
      })
    }

    const patchAuth = (user: string, pass: string) => {
      if (!user && !pass) {
        const rest = { ...first }
        delete rest.users
        onChange({
          ...outbound,
          settings: { ...settings, servers: [rest, ...servers.slice(1)] },
        })
        return
      }
      const nextUser = { ...firstUser, user, pass }
      const nextUsers = [nextUser, ...users.slice(1)]
      patchServer({ users: nextUsers })
    }

    return (
      <div className="grid gap-3 sm:grid-cols-2">
        <Field
          label="落地服务器地址 *"
          placeholder="203.0.113.10 或 landing.example.com"
          value={typeof first.address === 'string' ? first.address : ''}
          onChange={(e) => patchServer({ address: e.target.value.trim() })}
        />
        <Field
          label="端口 *"
          type="number"
          min={1}
          max={65535}
          placeholder="1080"
          value={typeof first.port === 'number' ? first.port : ''}
          onChange={(e) => patchServer({ port: Number(e.target.value) || 0 })}
        />
        <Field
          label="用户名（可选）"
          placeholder="留空为无认证"
          value={typeof firstUser.user === 'string' ? firstUser.user : ''}
          onChange={(e) =>
            patchAuth(e.target.value, typeof firstUser.pass === 'string' ? firstUser.pass : '')
          }
        />
        <SecretInput
          label="密码（可选）"
          placeholder="留空为无认证"
          disabled={disabled}
          value={typeof firstUser.pass === 'string' ? firstUser.pass : ''}
          onChange={(pass) =>
            patchAuth(typeof firstUser.user === 'string' ? firstUser.user : '', pass)
          }
        />
      </div>
    )
  }

  // Freedom helpers
  const renderFreedomFields = () => {
    const ds = typeof settings.domainStrategy === 'string' ? settings.domainStrategy : 'AsIs'
    return (
      <div className="grid gap-3 sm:grid-cols-2">
        <Field
          label="出口源 IP（可选）"
          placeholder="如 198.51.100.25"
          value={outbound.sendThrough ?? ''}
          onChange={(e) => {
            const val = e.target.value.trim()
            const next = { ...outbound }
            if (val) next.sendThrough = val
            else delete next.sendThrough
            onChange(next)
          }}
        />
        <SelectField
          label="出站域名策略"
          value={ds}
          onChange={(v) =>
            onChange({
              ...outbound,
              settings: { ...settings, domainStrategy: v },
            })
          }
          options={[
            { value: 'AsIs', label: 'AsIs（系统默认解析）' },
            { value: 'UseIP', label: 'UseIP（使用 Xray 内置 DNS 双栈）' },
            { value: 'UseIPv4', label: 'UseIPv4（优先内置 DNS IPv4）' },
            { value: 'UseIPv6', label: 'UseIPv6（优先内置 DNS IPv6）' },
            { value: 'ForceIPv4', label: 'ForceIPv4（强制仅 IPv4）' },
            { value: 'ForceIPv6', label: 'ForceIPv6（强制仅 IPv6）' },
          ]}
        />
      </div>
    )
  }

  // WireGuard helpers
  const renderWireGuardFields = () => {
    const addresses = Array.isArray(settings.address) ? (settings.address as string[]) : []
    const peers = Array.isArray(settings.peers) ? [...(settings.peers as Record<string, unknown>[])] : [{}]
    const peer0 = { ...(peers[0] ?? {}) }
    const reservedArr = Array.isArray(settings.reserved) ? (settings.reserved as number[]) : []

    const patchWG = (patch: Record<string, unknown>) => {
      onChange({
        ...outbound,
        settings: { ...settings, ...patch },
      })
    }

    const patchPeer0 = (patch: Record<string, unknown>) => {
      const nextPeer = { ...peer0, ...patch }
      patchWG({ peers: [nextPeer, ...peers.slice(1)] })
    }

    return (
      <div className="space-y-3">
        <div className="grid gap-3 sm:grid-cols-2">
          <SecretInput
            label="本机私钥 *"
            placeholder="Base64 WireGuard 私钥"
            disabled={disabled}
            value={typeof settings.secretKey === 'string' ? settings.secretKey : ''}
            onChange={(v) => patchWG({ secretKey: v.trim() })}
          />
          <Field
            label="隧道接口地址（逗号分隔）"
            placeholder="172.16.0.2/32, 2606:4700:110::2/128"
            value={addresses.join(', ')}
            onChange={(e) =>
              patchWG({
                address: e.target.value
                  .split(',')
                  .map((s) => s.trim())
                  .filter(Boolean),
              })
            }
          />
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field
            label="对端地址与端口 *"
            placeholder="engage.cloudflareclient.com:2408"
            value={typeof peer0.endpoint === 'string' ? peer0.endpoint : ''}
            onChange={(e) => patchPeer0({ endpoint: e.target.value.trim() })}
          />
          <Field
            label="对端公钥 *"
            placeholder="bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo="
            value={typeof peer0.publicKey === 'string' ? peer0.publicKey : ''}
            onChange={(e) => patchPeer0({ publicKey: e.target.value.trim() })}
          />
        </div>
        <details className="group/wg">
          <summary className="flex min-h-9 cursor-pointer list-none flex-wrap items-center gap-2 rounded text-sm font-medium text-soft hover:text-[var(--text)] [&::-webkit-details-marker]:hidden">
            <ChevronRight className="h-4 w-4 transition-transform group-open/wg:rotate-90" />
            WireGuard 高级设置
            {reservedError && <span className="text-[var(--danger)]">Reserved 格式有误</span>}
          </summary>
          <div className="grid gap-4 py-3 sm:grid-cols-3">
            <SecretInput
              label="预共享密钥"
              placeholder="留空不启用"
              disabled={disabled}
              value={typeof peer0.preSharedKey === 'string' ? peer0.preSharedKey : ''}
              onChange={(v) => {
                if (!v.trim()) {
                  const rest = { ...peer0 }
                  delete rest.preSharedKey
                  patchWG({ peers: [rest, ...peers.slice(1)] })
                } else {
                  patchPeer0({ preSharedKey: v.trim() })
                }
              }}
            />
            <Field
              label="MTU"
              type="number"
              placeholder="1280"
              value={typeof settings.mtu === 'number' ? settings.mtu : 1280}
              onChange={(e) => patchWG({ mtu: Number(e.target.value) || 1280 })}
            />
            <Field
              label="WARP Reserved"
              placeholder="3 个字节，如 0,0,0"
              value={reservedDraft ?? reservedArr.join(',')}
              error={reservedError || undefined}
              onChange={(e) => {
                const raw = e.target.value.trim()
                setReservedDraft(e.target.value)
                if (!raw) {
                  const rest = { ...settings }
                  delete rest.reserved
                  setReservedError('')
                  onRawErrorChange?.('')
                  onChange({ ...outbound, settings: rest })
                  return
                }
                const nums = raw.split(',').map((x) => x.trim() === '' ? NaN : Number(x.trim()))
                const error = wireGuardReservedError(nums)
                setReservedError(error)
                onRawErrorChange?.(error)
                if (!error) patchWG({ reserved: nums })
              }}
            />
          </div>
          {dialerField}
        </details>
      </div>
    )
  }

  // VLESS helpers
  const renderVlessFields = () => {
    const vnexts = Array.isArray(settings.vnext) ? [...(settings.vnext as Record<string, unknown>[])] : [{}]
    const vn0 = { ...(vnexts[0] ?? {}) }
    const users = Array.isArray(vn0.users) ? [...(vn0.users as Record<string, unknown>[])] : [{}]
    const user0: Record<string, unknown> = { encryption: 'none', ...(users[0] ?? {}) }

    const stream = (outbound.streamSettings && typeof outbound.streamSettings === 'object'
      ? outbound.streamSettings
      : {}) as Record<string, unknown>
    const network = typeof stream.network === 'string' ? stream.network : 'tcp'
    const security = typeof stream.security === 'string' ? stream.security : 'tls'
    const tlsSettings = (stream.tlsSettings && typeof stream.tlsSettings === 'object'
      ? stream.tlsSettings
      : {}) as Record<string, unknown>
    const realitySettings = (stream.realitySettings && typeof stream.realitySettings === 'object'
      ? stream.realitySettings
      : {}) as Record<string, unknown>

    const patchVnext = (patch: Record<string, unknown>) => {
      const nextVn = { ...vn0, ...patch }
      onChange({
        ...outbound,
        settings: { ...settings, vnext: [nextVn, ...vnexts.slice(1)] },
      })
    }

    const patchUser = (patch: Record<string, unknown>) => {
      const nextU = { ...user0, ...patch }
      patchVnext({ users: [nextU, ...users.slice(1)] })
    }

    const patchStream = (patch: Record<string, unknown>) => {
      onChange({
        ...outbound,
        streamSettings: { ...stream, ...patch },
      })
    }

    return (
      <div className="space-y-3">
        <div className="grid gap-3 sm:grid-cols-2">
          <Field
            label="远端服务器地址 *"
            placeholder="landing.example.com"
            value={typeof vn0.address === 'string' ? vn0.address : ''}
            onChange={(e) => patchVnext({ address: e.target.value.trim() })}
          />
          <Field
            label="端口 *"
            type="number"
            min={1}
            max={65535}
            placeholder="443"
            value={typeof vn0.port === 'number' ? vn0.port : ''}
            onChange={(e) => patchVnext({ port: Number(e.target.value) || 0 })}
          />
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <SecretInput
            label="用户 UUID *"
            placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
            disabled={disabled}
            value={typeof user0.id === 'string' ? user0.id : ''}
            onChange={(id) => patchUser({ id: id.trim() })}
          />
          <SelectField
            label="XTLS 流控"
            value={typeof user0.flow === 'string' ? user0.flow : ''}
            onChange={(flow) => {
              if (!flow) {
                const rest = { ...user0 }
                delete rest.flow
                patchVnext({ users: [rest, ...users.slice(1)] })
              } else {
                patchUser({ flow })
              }
            }}
            options={[
              { value: '', label: '无流控（WS / 标准 TLS）' },
              { value: 'xtls-rprx-vision', label: 'xtls-rprx-vision（Reality / TCP TLS）' },
            ]}
          />
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <SelectField
            label="传输方式"
            value={network}
            onChange={(v) => patchStream({ network: v })}
            options={[
              { value: 'tcp', label: 'TCP' },
              { value: 'ws', label: 'WebSocket (ws)' },
              { value: 'grpc', label: 'gRPC' },
            ]}
          />
          <SelectField
            label="传输安全"
            value={security}
            onChange={(v) => patchStream({ security: v })}
            options={[
              { value: 'tls', label: 'TLS' },
              { value: 'reality', label: 'Reality' },
              { value: 'none', label: '无加密（none）' },
            ]}
          />
        </div>
        {security === 'tls' && (
          <Field
            label="TLS SNI"
            placeholder="landing.example.com"
            value={typeof tlsSettings.serverName === 'string' ? tlsSettings.serverName : ''}
            onChange={(e) =>
              patchStream({ tlsSettings: { ...tlsSettings, serverName: e.target.value.trim() } })
            }
          />
        )}
        {security === 'reality' && (
          <div className="grid gap-3 sm:grid-cols-3">
            <Field
              label="Reality SNI"
              placeholder="www.microsoft.com"
              value={typeof realitySettings.serverName === 'string' ? realitySettings.serverName : ''}
              onChange={(e) =>
                patchStream({
                  realitySettings: { ...realitySettings, serverName: e.target.value.trim() },
                })
              }
            />
            <Field
              label="Reality 公钥"
              placeholder="远端节点公钥"
              value={typeof realitySettings.publicKey === 'string' ? realitySettings.publicKey : ''}
              onChange={(e) =>
                patchStream({
                  realitySettings: { ...realitySettings, publicKey: e.target.value.trim() },
                })
              }
            />
            <Field
              label="ShortId"
              placeholder="如 abcd"
              value={typeof realitySettings.shortId === 'string' ? realitySettings.shortId : ''}
              onChange={(e) =>
                patchStream({
                  realitySettings: { ...realitySettings, shortId: e.target.value.trim() },
                })
              }
            />
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="space-y-4 [&_label]:text-sm">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div role="group" aria-label="出站编辑方式" className="inline-flex shrink-0 rounded-md border border-[var(--border)] bg-[var(--panel)] p-0.5 text-sm">
          <button
            type="button"
            disabled={!structuredSupported}
            title={structuredSupported ? undefined : `${outbound.protocol} 仅支持 JSON 编辑`}
            aria-pressed={activeMode === 'form'}
            onClick={() => switchMode('form')}
            className={`inline-flex items-center gap-1 rounded px-2.5 py-1 font-medium transition ${
              activeMode === 'form'
                ? 'bg-[var(--accent)] text-[var(--accent-contrast)]'
                : 'text-soft hover:text-[var(--text)] disabled:opacity-40'
            }`}
          >
            <SlidersHorizontal className="h-3.5 w-3.5" />
            表单
          </button>
          <button
            type="button"
            aria-pressed={activeMode === 'json'}
            onClick={() => switchMode('json')}
            className={`inline-flex items-center gap-1 rounded px-2.5 py-1 font-medium transition ${
              activeMode === 'json'
                ? 'bg-[var(--accent)] text-[var(--accent-contrast)]'
                : 'text-soft hover:text-[var(--text)]'
            }`}
          >
            <Code2 className="h-3.5 w-3.5" />
            JSON
          </button>
        </div>
        {action}
      </div>

      {activeMode === 'form' ? (
        <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-3">
            <Field
              label="出站 Tag *"
              value={lockedTag ?? outbound.tag}
              disabled={disabled || !!lockedTag}
              error={tagError || undefined}
              placeholder="如 hk-landing / warp-egress"
              onChange={(e) => onChange({ ...outbound, tag: e.target.value.trim() })}
            />
            <SelectField
              label="出站协议 *"
              value={outbound.protocol}
              onChange={updateProtocol}
              options={ALL_PROTOCOL_OPTIONS}
            />
            <Field
              label="面板备注"
              placeholder="仅在面板中显示"
              value={outbound.remark ?? ''}
              onChange={(e) => {
                const next = { ...outbound }
                if (e.target.value) next.remark = e.target.value
                else delete next.remark
                onChange(next)
              }}
            />
          </div>

          {(outbound.protocol === 'socks' || outbound.protocol === 'http') && renderProxyServerFields()}
          {outbound.protocol === 'freedom' && renderFreedomFields()}
          {outbound.protocol === 'wireguard' && renderWireGuardFields()}
          {outbound.protocol === 'vless' && renderVlessFields()}

          {outbound.protocol !== 'wireguard' && dialerField}
        </div>
      ) : (
        <div className="space-y-2">
          <JsonEditor
            ariaLabel={`自定义出站 ${outbound.tag || ''} JSON`}
            value={rawText}
            readOnly={disabled}
            minHeight="200px"
            onChange={(text) => {
              try {
                const parsed = parseSingleObject<CustomOutbound>(text, '自定义出站')
                const fixed: CustomOutbound = {
                  ...parsed,
                  tag: lockedTag ?? (typeof parsed.tag === 'string' ? parsed.tag : ''),
                  protocol: typeof parsed.protocol === 'string' ? parsed.protocol : '',
                }
                if (fixed.protocol === 'wireguard') {
                  const error = wireGuardReservedError(fixed.settings?.reserved)
                  if (error) throw new Error(error)
                }
                setRawDraft({ text, source: JSON.stringify(fixed) })
                setRawError('')
                onRawErrorChange?.('')
                onChange(fixed)
              } catch (e) {
                setRawDraft({ text, source: serialized })
                const msg = (e as Error).message
                setRawError(msg)
                onRawErrorChange?.(msg)
              }
            }}
          />
          {rawError && (
            <p role="alert" className="text-sm text-[var(--danger)]">
              {rawError}
            </p>
          )}
        </div>
      )}
    </div>
  )
}

function OutboundCard({
  outbound,
  refCount,
  error,
  initiallyOpen,
  children,
}: {
  outbound: CustomOutbound
  refCount: number
  error: string
  initiallyOpen: boolean
  children: ReactNode
}) {
  const [open, setOpen] = useState(initiallyOpen)
  const statusId = useId()
  const name = outbound.tag || '未命名出站'
  const dialer = extractDialerProxy(outbound)
  return (
    <details
      open={open}
      onToggle={(event) => setOpen(event.currentTarget.open)}
      className="group/card rounded-lg border border-[var(--border)] bg-[var(--panel-muted)]"
    >
      <summary
        aria-label={`出站 ${name}`}
        aria-describedby={statusId}
        className="flex min-h-14 cursor-pointer list-none flex-wrap items-center gap-x-3 gap-y-2 rounded-lg px-4 py-3 hover:bg-[var(--panel-strong)] sm:px-5 [&::-webkit-details-marker]:hidden"
      >
        <ChevronRight className="h-4 w-4 shrink-0 text-soft transition-transform group-open/card:rotate-90" />
        <span className="break-all font-mono text-base font-semibold">{name}</span>
        <span id={statusId} className="contents">
          <Badge label={outbound.protocol.toUpperCase()} variant="blue" />
          <span className="text-sm text-soft">{refCount > 0 ? `被引用 ${refCount} 次` : '未被引用'}</span>
          {dialer && (
            <span className="inline-flex min-w-0 items-center gap-1 break-all text-sm text-soft">
              <Link2 className="h-3.5 w-3.5 shrink-0" />
              前置 {dialer}
            </span>
          )}
          {error && <span className="text-sm font-medium text-[var(--danger)]">{error}</span>}
        </span>
      </summary>
      <div className="px-4 pb-4 sm:px-5 sm:pb-5">{children}</div>
    </details>
  )
}

export function OutboundListEditor({
  outbounds,
  rules,
  balancers = [],
  disabled = false,
  onChange,
  onPendingErrorChange,
}: {
  outbounds: CustomOutbound[]
  rules: RoutingRuleItem[]
  balancers?: BalancerItem[]
  disabled?: boolean
  onChange: (next: CustomOutbound[]) => void
  onPendingErrorChange?: (hasError: boolean) => void
}) {
  const [newProtocol, setNewProtocol] = useState<string>('socks')
  const [snippetsOpen, setSnippetsOpen] = useState(false)
  const [cardErrors, setCardErrors] = useState<Record<number, string>>({})
  const [newCardKey, setNewCardKey] = useState<number>()

  const updateCardError = (idx: number, err: string) => {
    const next = { ...cardErrors }
    if (err) next[idx] = err
    else delete next[idx]
    setCardErrors(next)
    onPendingErrorChange?.(Object.keys(next).length > 0)
  }

  const appendOutbound = (item: CustomOutbound) => {
    setNewCardKey(editorItemKey(item))
    onChange([...outbounds, item])
  }

  const addOutbound = () => {
    let idx = outbounds.length + 1
    const existingTags = new Set([...outbounds.map((o) => o.tag), ...balancers.map((b) => b.tag)])
    let candidate = createDefaultOutbound(newProtocol, idx)
    while (existingTags.has(candidate.tag)) {
      idx += 1
      candidate = createDefaultOutbound(newProtocol, idx)
    }
    appendOutbound(candidate)
  }

  const allTags = outbounds.map((o) => o.tag)

  return (
    <div className="space-y-5">
      <details className="group/builtins border-b border-[var(--border)] pb-3">
        <summary className="flex min-h-9 cursor-pointer list-none items-center gap-2 rounded text-sm font-medium hover:text-[var(--accent)] [&::-webkit-details-marker]:hidden">
          <ShieldCheck className="h-4 w-4 text-[var(--accent)]" />
          系统内置出站
          <ChevronRight className="h-4 w-4 text-soft transition-transform group-open/builtins:rotate-90" />
        </summary>
        <div className="space-y-2 py-2 pl-6 text-sm leading-6 text-soft">
          <ul className="flex flex-wrap gap-x-5 gap-y-1">
            {BUILTIN_OUTBOUNDS.map((item) => <li key={item.tag}>{item.label}</li>)}
          </ul>
          <p>系统自动生成内置出站，只下发实际使用的自定义出站。</p>
        </div>
      </details>

      <div className="space-y-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div className="w-full sm:w-64 [&_label]:text-sm">
            <SelectField
              label="出站类型"
              value={newProtocol}
              onChange={setNewProtocol}
              options={STRUCTURED_OUTBOUND_PROTOCOLS.map((p) => ({ value: p.value, label: p.label }))}
            />
          </div>
          <div className="flex flex-wrap gap-2">
            <Btn
              variant="secondary"
              disabled={disabled || outbounds.length >= 32}
              onClick={() => setSnippetsOpen(!snippetsOpen)}
            >
              <Sparkles className="h-4 w-4" />
              场景出站片段
            </Btn>
            <Btn
              variant="secondary"
              disabled={disabled || outbounds.length >= 32}
              onClick={addOutbound}
            >
              <Plus className="h-4 w-4" />
              添加自定义出站
              <span className="tabular-nums text-soft">{outbounds.length}/32</span>
            </Btn>
          </div>
        </div>

        {snippetsOpen && (
          <div className="grid gap-2.5 border-t border-[var(--border)] pt-3 sm:grid-cols-3">
            {OUTBOUND_SNIPPETS.map((snippet) => (
              <button
                key={snippet.key}
                type="button"
                disabled={disabled || outbounds.length >= 32}
                onClick={() => {
                  appendOutbound(snippet.build(allTags))
                  setSnippetsOpen(false)
                }}
                className="flex flex-col items-start gap-1 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] p-3 text-left transition hover:border-[var(--accent)] hover:bg-[var(--panel-strong)] disabled:opacity-50"
              >
                <span className="text-sm font-semibold text-[var(--accent)]">{snippet.name}</span>
                <span className="text-sm leading-5 text-soft">{snippet.description}</span>
              </button>
            ))}
          </div>
        )}
      </div>

      {outbounds.length === 0 ? (
        <div className="rounded-lg border border-dashed border-[var(--border-strong)] bg-[var(--panel-muted)] p-6 text-center text-sm text-soft">
          尚未添加自定义出站
        </div>
      ) : (
        <div className="space-y-4">
          {outbounds.map((item, idx) => {
            const key = editorItemKey(item)
            const refCount = countOutboundReferences(item.tag, rules, outbounds, balancers)
            const duplicateCount = outbounds.filter((o) => o.tag && o.tag === item.tag).length
            const tagError = duplicateCount > 1 ? 'Tag 重复' : validateOutboundTag(item.tag)
            return (
              <OutboundCard
                key={key}
                outbound={item}
                refCount={refCount}
                error={tagError || (cardErrors[key] ? '输入有误' : '')}
                initiallyOpen={idx === 0 || key === newCardKey}
              >
                <SingleOutboundFields
                  action={
                    <Btn
                      variant="ghost"
                      disabled={disabled || refCount > 0}
                      title={refCount > 0 ? '请先解除出站引用' : '删除此自定义出站'}
                      onClick={() => {
                        updateCardError(key, '')
                        onChange(outbounds.filter((_, i) => i !== idx))
                      }}
                      className="h-9 px-2 hover:text-[var(--danger)]"
                    >
                      <Trash2 className="h-4 w-4" />
                      {refCount > 0 ? '使用中禁删' : '删除'}
                    </Btn>
                  }
                  outbound={item}
                  peerOutboundTags={allTags}
                  disabled={disabled}
                  onRawErrorChange={(err) => updateCardError(key, err)}
                  onChange={(updated) => {
                    const next = [...outbounds]
                    next[idx] = retainEditorItemKey(item, updated)
                    onChange(next)
                  }}
                />
              </OutboundCard>
            )
          })}
        </div>
      )}
    </div>
  )
}
