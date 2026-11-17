import { useState } from 'react'
import { Code2, KeyRound, Link2, Plus, ShieldCheck, SlidersHorizontal, Sparkles, Trash2 } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Btn, Field, SelectField } from '@/components/ui/Form'
import { JsonEditor } from '@/components/ui/JsonEditor'
import { OUTBOUND_SNIPPETS } from '@/lib/routing-snippets'
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
import type { CustomOutbound, RoutingRuleItem } from '@/lib/routing-model'

const ALL_PROTOCOL_OPTIONS = [
  ...STRUCTURED_OUTBOUND_PROTOCOLS,
  { value: 'trojan', label: 'Trojan（JSON 模式）' },
  { value: 'shadowsocks', label: 'Shadowsocks（JSON 模式）' },
  { value: 'vmess', label: 'VMess（JSON 模式）' },
  { value: 'blackhole', label: 'Blackhole（JSON 模式）' },
  { value: 'loopback', label: 'Loopback（JSON 模式）' },
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
        <span className="text-[12px] font-medium text-soft">{label}</span>
        <div className="flex h-10 items-center justify-between gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] px-3 text-xs">
          <span className="inline-flex items-center gap-1.5 text-soft">
            <KeyRound className="h-3.5 w-3.5 text-[var(--accent)]" />
            已加密保存（未修改将复用原密文）
          </span>
          {!disabled && (
            <button
              type="button"
              onClick={() => {
                setEditingCipher(true)
                onChange('')
              }}
              className="text-xs font-medium text-[var(--accent)] hover:underline"
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
  lockedTag,
  peerOutboundTags = [],
  disabled = false,
  onChange,
  onRawErrorChange,
}: {
  outbound: CustomOutbound
  lockedTag?: string
  peerOutboundTags?: string[]
  disabled?: boolean
  onChange: (next: CustomOutbound) => void
  onRawErrorChange?: (err: string) => void
}) {
  const structuredSupported = isStructuredOutboundProtocol(outbound.protocol)
  const [mode, setMode] = useState<'form' | 'json'>(structuredSupported ? 'form' : 'json')
  const [rawText, setRawText] = useState(() => JSON.stringify(outbound, null, 2))
  const [rawError, setRawError] = useState('')

  const activeMode = !structuredSupported ? 'json' : mode
  const tagError = lockedTag ? '' : validateOutboundTag(outbound.tag)

  const switchMode = (target: 'form' | 'json') => {
    if (target === 'json') {
      setRawText(JSON.stringify(outbound, null, 2))
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
      if (!isStructuredOutboundProtocol(fixed.protocol)) {
        setRawError(`协议 ${fixed.protocol} 暂不支持表单模式，请继续使用 JSON 编辑或切换为受支持协议。`)
        return
      }
      onChange(fixed)
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
    { value: '', label: '不使用前置链式代理（节点直接拨号）' },
    { value: 'direct', label: 'direct（默认直连）' },
    { value: 'direct-v4', label: 'direct-v4（强制 IPv4 拨号）' },
    { value: 'direct-v6', label: 'direct-v6（强制 IPv6 拨号）' },
    ...peerOutboundTags
      .filter((t) => t && t !== outbound.tag)
      .map((t) => ({ value: t, label: `${t}（链式出站）` })),
  ]

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
      setRawText(JSON.stringify(next, null, 2))
      setMode('json')
    }
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
          label="密码（可选，AES-GCM 加密存储）"
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
          label="指定出口源 IP（sendThrough，可选）"
          placeholder="如多 IP 机器副 IP：198.51.100.25"
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
          label="出站域名策略（domainStrategy）"
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
    const reservedArr = Array.isArray(peer0.reserved) ? (peer0.reserved as number[]) : []

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
            label="本机私钥（secretKey *，AES-GCM 加密存储）"
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
            label="对端 Endpoint（含端口） *"
            placeholder="engage.cloudflareclient.com:2408"
            value={typeof peer0.endpoint === 'string' ? peer0.endpoint : ''}
            onChange={(e) => patchPeer0({ endpoint: e.target.value.trim() })}
          />
          <Field
            label="对端公钥（publicKey） *"
            placeholder="bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo="
            value={typeof peer0.publicKey === 'string' ? peer0.publicKey : ''}
            onChange={(e) => patchPeer0({ publicKey: e.target.value.trim() })}
          />
        </div>
        <div className="grid gap-3 sm:grid-cols-3">
          <SecretInput
            label="预共享密钥（preSharedKey，可选）"
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
            label="WARP Reserved（可选，如 0,0,0）"
            placeholder="留空或填 3 个字节整数"
            value={reservedArr.join(',')}
            onChange={(e) => {
              const raw = e.target.value.trim()
              if (!raw) {
                const rest = { ...peer0 }
                delete rest.reserved
                patchWG({ peers: [rest, ...peers.slice(1)] })
                return
              }
              const nums = raw
                .split(',')
                .map((x) => Number(x.trim()))
                .filter((n) => !Number.isNaN(n))
              patchPeer0({ reserved: nums })
            }}
          />
        </div>
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
            label="用户 UUID（id *，AES-GCM 加密存储）"
            placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
            disabled={disabled}
            value={typeof user0.id === 'string' ? user0.id : ''}
            onChange={(id) => patchUser({ id: id.trim() })}
          />
          <SelectField
            label="XTLS 流控（flow）"
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
            label="传输层（network）"
            value={network}
            onChange={(v) => patchStream({ network: v })}
            options={[
              { value: 'tcp', label: 'TCP' },
              { value: 'ws', label: 'WebSocket (ws)' },
              { value: 'grpc', label: 'gRPC' },
            ]}
          />
          <SelectField
            label="传输安全（security）"
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
            label="TLS SNI（serverName）"
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
              label="Reality SNI（serverName）"
              placeholder="www.microsoft.com"
              value={typeof realitySettings.serverName === 'string' ? realitySettings.serverName : ''}
              onChange={(e) =>
                patchStream({
                  realitySettings: { ...realitySettings, serverName: e.target.value.trim() },
                })
              }
            />
            <Field
              label="Reality 公钥（publicKey）"
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
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-xs text-soft">
          {structuredSupported
            ? '支持结构化表单与原始 JSON 无损切换，JSON 中添加的高级字段会自动保留。'
            : `当前协议（${outbound.protocol}）使用单条 JSON 模式配置。`}
        </span>
        <div className="inline-flex rounded-md border border-[var(--border)] bg-[var(--panel)] p-0.5 text-xs">
          <button
            type="button"
            disabled={!structuredSupported}
            onClick={() => switchMode('form')}
            className={`inline-flex items-center gap-1 rounded px-2.5 py-1 font-medium transition ${
              activeMode === 'form'
                ? 'bg-[var(--accent)] text-white'
                : 'text-soft hover:text-[var(--text)] disabled:opacity-40'
            }`}
          >
            <SlidersHorizontal className="h-3.5 w-3.5" />
            表单
          </button>
          <button
            type="button"
            onClick={() => switchMode('json')}
            className={`inline-flex items-center gap-1 rounded px-2.5 py-1 font-medium transition ${
              activeMode === 'json'
                ? 'bg-[var(--accent)] text-white'
                : 'text-soft hover:text-[var(--text)]'
            }`}
          >
            <Code2 className="h-3.5 w-3.5" />
            JSON
          </button>
        </div>
      </div>

      {activeMode === 'form' ? (
        <div className="space-y-3">
          <div className="grid gap-3 sm:grid-cols-3">
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
              label="面板备注（下发时自动剥离）"
              placeholder="如 香港解锁落地"
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

          <div className="pt-1">
            <SelectField
              label="前置链式拨号出站（sockopt.dialerProxy，可选）"
              value={extractDialerProxy(outbound)}
              onChange={(dp) => onChange(patchDialerProxy(outbound, dp))}
              options={dialerOptions}
            />
          </div>
        </div>
      ) : (
        <div className="space-y-2">
          <JsonEditor
            ariaLabel={`自定义出站 ${outbound.tag || ''} JSON`}
            value={rawText}
            readOnly={disabled}
            minHeight="200px"
            onChange={(text) => {
              setRawText(text)
              try {
                const parsed = parseSingleObject<CustomOutbound>(text, '自定义出站')
                const fixed: CustomOutbound = {
                  ...parsed,
                  tag: lockedTag ?? (typeof parsed.tag === 'string' ? parsed.tag : ''),
                  protocol: typeof parsed.protocol === 'string' ? parsed.protocol : '',
                }
                setRawError('')
                onRawErrorChange?.('')
                onChange(fixed)
              } catch (e) {
                const msg = (e as Error).message
                setRawError(msg)
                onRawErrorChange?.(msg)
              }
            }}
          />
          {rawError && (
            <p role="alert" className="text-xs text-[var(--danger)]">
              {rawError}
            </p>
          )}
        </div>
      )}
    </div>
  )
}

export function OutboundListEditor({
  outbounds,
  rules,
  disabled = false,
  onChange,
  onPendingErrorChange,
}: {
  outbounds: CustomOutbound[]
  rules: RoutingRuleItem[]
  disabled?: boolean
  onChange: (next: CustomOutbound[]) => void
  onPendingErrorChange?: (hasError: boolean) => void
}) {
  const [newProtocol, setNewProtocol] = useState<string>('socks')
  const [snippetsOpen, setSnippetsOpen] = useState(false)
  const [cardErrors, setCardErrors] = useState<Record<number, string>>({})

  const updateCardError = (idx: number, err: string) => {
    const next = { ...cardErrors }
    if (err) next[idx] = err
    else delete next[idx]
    setCardErrors(next)
    onPendingErrorChange?.(Object.keys(next).length > 0)
  }

  const addOutbound = () => {
    let idx = outbounds.length + 1
    const existingTags = new Set(outbounds.map((o) => o.tag))
    let candidate = createDefaultOutbound(newProtocol, idx)
    while (existingTags.has(candidate.tag)) {
      idx += 1
      candidate = createDefaultOutbound(newProtocol, idx)
    }
    onChange([...outbounds, candidate])
  }

  const allTags = outbounds.map((o) => o.tag)

  return (
    <div className="space-y-4">
      <section className="rounded-lg border border-[var(--border)] bg-[var(--panel-muted)] p-4">
        <div className="flex items-start gap-2.5">
          <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-[var(--accent)]" />
          <div className="space-y-1.5 text-xs">
            <p className="font-semibold text-[var(--text)]">系统内置出站（自动受控生成，无需重复定义）</p>
            <div className="flex flex-wrap gap-2 pt-0.5">
              {BUILTIN_OUTBOUNDS.map((item) => (
                <span
                  key={item.tag}
                  className="rounded border border-[var(--border)] bg-[var(--panel-strong)] px-2 py-1 font-mono text-[11px]"
                >
                  {item.label}
                </span>
              ))}
            </div>
            <p className="text-soft">
              未被任何启用规则或链式代理（dialerProxy）引用的自定义出站，在生成节点最终配置时会自动剔除。
            </p>
          </div>
        </div>
      </section>

      <div className="space-y-3 rounded-lg border border-[var(--border)] bg-[var(--panel)] p-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div className="w-full sm:w-72">
            <SelectField
              label="选择要添加的出站类型"
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
              添加自定义出站（{outbounds.length}/32）
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
                  onChange([...outbounds, snippet.build(allTags)])
                  setSnippetsOpen(false)
                }}
                className="flex flex-col items-start gap-1 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] p-3 text-left transition hover:border-[var(--accent)] hover:bg-[var(--panel-strong)] disabled:opacity-50"
              >
                <span className="text-xs font-semibold text-[var(--accent)]">{snippet.name}</span>
                <span className="text-[11px] leading-4 text-soft">{snippet.description}</span>
              </button>
            ))}
          </div>
        )}
      </div>

      {outbounds.length === 0 ? (
        <div className="rounded-lg border border-dashed border-[var(--border-strong)] bg-[var(--panel-muted)] p-6 text-center text-sm text-soft">
          当前模板未定义自定义出站。如需配置外部落地机、Cloudflare WARP 出口或多 IP 指定源地址，请点击上方“添加自定义出站”。
        </div>
      ) : (
        <div className="space-y-4">
          {outbounds.map((item, idx) => {
            const refCount = countOutboundReferences(item.tag, rules, outbounds)
            const duplicateCount = outbounds.filter((o) => o.tag && o.tag === item.tag).length
            return (
              <div
                key={idx}
                className="space-y-3 rounded-lg border border-[var(--border)] bg-[var(--panel-muted)] p-4"
              >
                <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border)] pb-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-sm font-semibold text-[var(--text)]">
                      {item.tag || '未命名出站'}
                    </span>
                    <Badge label={item.protocol.toUpperCase()} variant="blue" />
                    <Badge
                      label={refCount > 0 ? `被引用 ${refCount} 次` : '未被引用'}
                      variant={refCount > 0 ? 'green' : 'gray'}
                    />
                    {extractDialerProxy(item) && (
                      <span className="inline-flex items-center gap-1 text-xs text-soft">
                        <Link2 className="h-3.5 w-3.5" />
                        前置: {extractDialerProxy(item)}
                      </span>
                    )}
                    {duplicateCount > 1 && (
                      <span className="text-xs font-medium text-[var(--danger)]">Tag 重复</span>
                    )}
                  </div>
                  <Btn
                    variant="danger"
                    disabled={disabled || refCount > 0}
                    title={
                      refCount > 0
                        ? '该出站正被分流规则或 dialerProxy 引用，请先解除引用后再删除'
                        : '删除此自定义出站'
                    }
                    onClick={() => {
                      updateCardError(idx, '')
                      onChange(outbounds.filter((_, i) => i !== idx))
                    }}
                    className="h-8 px-3 text-xs"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    {refCount > 0 ? '使用中禁删' : '删除'}
                  </Btn>
                </div>

                <SingleOutboundFields
                  outbound={item}
                  peerOutboundTags={allTags}
                  disabled={disabled}
                  onRawErrorChange={(err) => updateCardError(idx, err)}
                  onChange={(updated) => {
                    const next = [...outbounds]
                    next[idx] = updated
                    onChange(next)
                  }}
                />
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
