import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, ArrowLeftRight, Copy, PencilLine, Sparkles, Terminal } from 'lucide-react'
import { installApi, nodeApi, type InstallToken } from '@/lib/api'
import type { Node } from '@/types'
import { copyText } from '@/lib/clipboard'
import { Modal } from '@/components/ui/Modal'
import { Field, Btn, FieldGroup, SelectField } from '@/components/ui/Form'
import { pushToast } from '@/lib/notify'

type Step = 'mode' | 'ip' | 'form' | 'waiting' | 'success' | 'expired'

interface Props {
  open: boolean
  onClose: () => void
  // 节点注册成功后由父组件刷新节点列表
  onRegistered?: (nodeId: number) => void
  // 更换服务器模式：给该现有节点换新机（分组/协议密钥/覆盖全保留）。
  // 不传则为普通一键接入（新建节点）。
  replaceNode?: Node | null
}

const TTL_OPTIONS = [
  { value: '600', label: '10 分钟（默认）' },
  { value: '3600', label: '1 小时' },
  { value: '86400', label: '24 小时' },
]

// OneClickInstallDialog 节点一键接入对话框
// 流程：填表单 → 后端生成 token → 显示 curl 命令带轮询 → 节点上报回 panel 成功 → 关闭
export function OneClickInstallDialog({ open, onClose, onRegistered, replaceNode }: Props) {
  const isReplace = !!replaceNode
  const [step, setStep] = useState<Step>('form')
  const [submitting, setSubmitting] = useState(false)
  const [token, setToken] = useState<InstallToken | null>(null)
  const [copied, setCopied] = useState(false)
  const successFiredRef = useRef(false)

  const [form, setForm] = useState({
    name: '',
    region: '',
    owner: '',
    remark: '',
    ssh_user: 'root',
    ssh_port: '22',
    ttl_seconds: '600',
  })
  const [newIP, setNewIP] = useState('')
  const [err, setErr] = useState('')

  // 重置：每次打开对话框回到起始步骤（更换模式先选方式，普通模式直接进表单）
  useEffect(() => {
    if (open) {
      setStep(replaceNode ? 'mode' : 'form')
      setToken(null)
      setCopied(false)
      setErr('')
      setNewIP('')
      successFiredRef.current = false
      if (replaceNode) {
        // SSH 参数带出现有节点值作为可改初值
        setForm((p) => ({
          ...p,
          ssh_user: replaceNode.ssh_user || 'root',
          ssh_port: String(replaceNode.ssh_port || 22),
        }))
      }
    }
  }, [open, replaceNode])

  // 倒计时显示
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    if (step !== 'waiting') return
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [step])

  // 轮询 token 状态（仅 waiting 态）
  useQuery({
    queryKey: ['install-token-poll', token?.token],
    queryFn: async () => {
      if (!token) return null
      const r = await installApi.get(token.token)
      const latest = r.data.data!
      // 成功
      if (latest.used && latest.node_id) {
        if (!successFiredRef.current) {
          successFiredRef.current = true
          setToken(latest) // 保留 reachable 等字段到 success 态展示
          setStep('success')
          // 连通时 3 秒后自动关闭；不通时让用户多看一会再关
          const closeDelay = latest.reachable === false ? 7000 : 3000
          window.setTimeout(() => {
            onRegistered?.(latest.node_id!)
            onClose()
          }, closeDelay)
        }
      }
      return latest
    },
    enabled: open && step === 'waiting' && !!token,
    refetchInterval: 2000,
    // 404 / 410 表示 token 已被清理或过期：交给倒计时分支统一处理
    retry: false,
  })

  // token 过期检测（轮询期间）
  useEffect(() => {
    if (step !== 'waiting' || !token) return
    if (new Date(token.expires_at).getTime() <= now) {
      setStep('expired')
    }
  }, [step, token, now])

  const handleCreate = async () => {
    setErr('')
    if (!isReplace && !form.name.trim()) {
      setErr('节点名不能为空')
      return
    }
    setSubmitting(true)
    try {
      const res = await installApi.create({
        name: isReplace ? undefined : form.name.trim(),
        region: isReplace ? undefined : form.region.trim() || undefined,
        owner: isReplace ? undefined : form.owner.trim() || undefined,
        remark: isReplace ? undefined : form.remark.trim() || undefined,
        replace_node_id: replaceNode?.id,
        ssh_user: form.ssh_user.trim() || 'root',
        ssh_port: Number(form.ssh_port) || 22,
        ttl_seconds: Number(form.ttl_seconds) || 600,
        panel_url: window.location.origin,
      })
      setToken(res.data.data!)
      setStep('waiting')
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  // 仅更换 IP：直接更新节点记录（后端自动清理旧 known_hosts 并标记待同步）
  const handleUpdateIP = async () => {
    if (!replaceNode) return
    setErr('')
    const ip = newIP.trim()
    if (!ip) {
      setErr('新 IP 不能为空')
      return
    }
    if (ip === replaceNode.ip) {
      setErr('新 IP 与当前 IP 相同')
      return
    }
    setSubmitting(true)
    try {
      await nodeApi.update(replaceNode.id, { ip })
      pushToast({
        title: 'IP 已更新',
        description: replaceNode.domain
          ? `请将域名 ${replaceNode.domain} 的 DNS 解析指向 ${ip}，然后在节点列表点「同步」推送配置。`
          : '节点已标记待同步，请在节点列表点「同步」推送配置。',
        variant: 'success',
      })
      onRegistered?.(replaceNode.id)
      onClose()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  const handleCopyCommand = async () => {
    if (!token?.curl_command) return
    const ok = await copyText(token.curl_command)
    if (ok) {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } else {
      pushToast({
        title: '复制失败',
        description: '请手动选中命令并按 Ctrl+C 复制',
        variant: 'warning',
      })
    }
  }

  const handleRegenerate = () => {
    setStep('form')
    setToken(null)
    setErr('')
  }

  const handleCancel = () => {
    // 关闭对话框；未使用的 token 由后台定时清理或下次重新生成时自然失效
    onClose()
  }

  const remainingSec = token
    ? Math.max(0, Math.floor((new Date(token.expires_at).getTime() - now) / 1000))
    : 0
  const remainingLabel = remainingSec > 0
    ? `${Math.floor(remainingSec / 60).toString().padStart(2, '0')}:${(remainingSec % 60).toString().padStart(2, '0')}`
    : '00:00'

  return (
    <Modal
      open={open}
      onClose={handleCancel}
      title={isReplace ? `更换服务器 · ${replaceNode!.name}` : '一键接入节点'}
      size="lg"
      footer={
        step === 'mode' ? (
          <Btn variant="secondary" onClick={handleCancel}>取消</Btn>
        ) : step === 'ip' ? (
          <>
            <Btn variant="secondary" onClick={() => { setStep('mode'); setErr('') }}>返回</Btn>
            <Btn loading={submitting} onClick={handleUpdateIP}>保存新 IP</Btn>
          </>
        ) : step === 'form' ? (
          <>
            {isReplace ? (
              <Btn variant="secondary" onClick={() => { setStep('mode'); setErr('') }}>返回</Btn>
            ) : (
              <Btn variant="secondary" onClick={handleCancel}>取消</Btn>
            )}
            <Btn loading={submitting} onClick={handleCreate}>
              <Sparkles className="h-4 w-4" />
              生成接入命令
            </Btn>
          </>
        ) : step === 'expired' ? (
          <>
            <Btn variant="secondary" onClick={handleCancel}>关闭</Btn>
            <Btn onClick={handleRegenerate}>重新生成</Btn>
          </>
        ) : (
          <Btn variant="secondary" onClick={handleCancel}>关闭</Btn>
        )
      }
    >
      {step === 'mode' && replaceNode && (
        <div className="space-y-4">
          <p className="text-sm leading-6 text-soft">
            当前地址 <code className="wrap-anywhere">{replaceNode.ip}</code>
            {replaceNode.domain && <>（<code className="wrap-anywhere">{replaceNode.domain}</code>）</>}。两种方式都沿用原有分组、协议密钥、端口、SNI 和订阅链接。
          </p>
          <button
            type="button"
            onClick={() => { setStep('ip'); setErr('') }}
            className="flex w-full items-start gap-3 rounded-xl border border-[var(--border)] bg-[var(--panel-muted)] p-4 text-left transition hover:border-[var(--accent)]"
          >
            <PencilLine className="mt-0.5 h-5 w-5 shrink-0 text-[var(--accent)]" />
            <span>
              <span className="block text-sm font-semibold">仅更换 IP</span>
              <span className="mt-1 block text-sm text-soft">
                原服务器更改地址后手动同步{replaceNode.domain ? '，并更新域名解析' : ''}。
              </span>
            </span>
          </button>
          <button
            type="button"
            onClick={() => { setStep('form'); setErr('') }}
            className="flex w-full items-start gap-3 rounded-xl border border-[var(--border)] bg-[var(--panel-muted)] p-4 text-left transition hover:border-[var(--accent)]"
          >
            <ArrowLeftRight className="mt-0.5 h-5 w-5 shrink-0 text-[var(--accent)]" />
            <span>
              <span className="block text-sm font-semibold">更换服务器（重装接入）</span>
              <span className="mt-1 block text-sm text-soft">
                在新服务器运行一次性接入命令。注册并通过 SSH 检查后，面板会触发配置同步。
              </span>
            </span>
          </button>
        </div>
      )}

      {step === 'ip' && replaceNode && (
        <div className="space-y-4 [&_label]:text-sm">
          <Field label="当前 IP" value={replaceNode.ip} disabled readOnly />
          <Field label="新 IP *" value={newIP} onChange={(e) => setNewIP(e.target.value)} placeholder="如：34.21.157.200" />
          <p className={replaceNode.domain ? 'text-sm leading-6 text-[var(--warning)]' : 'text-sm leading-6 text-soft'}>
            {replaceNode.domain && <><AlertTriangle className="mr-1 inline h-4 w-4" />请将 <code className="wrap-anywhere">{replaceNode.domain}</code> 的 DNS 解析指向新 IP。</>}
            保存后，请在节点列表点击「同步」推送配置。
          </p>
          {err && <p className="text-sm text-[var(--danger)]">{err}</p>}
        </div>
      )}

      {step === 'form' && (
        <div className="space-y-4 [&_label]:text-sm [&_h4]:text-base">
          <p className="text-sm leading-6 text-soft">
            命令会安装「节点与同步」中指定的 Xray 版本，{replaceNode
              ? <>并将新服务器注册到「{replaceNode.name}」。原有节点 ID、密钥和订阅继续使用。</>
              : '并将服务器注册为新节点。'}
          </p>
          {!isReplace && (
            <FieldGroup title="节点信息">
              <Field label="节点名 *" value={form.name} onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))} placeholder="如：tw03-lk" />
              <div className="grid gap-4 md:grid-cols-2">
                <Field label="地区" value={form.region} onChange={(e) => setForm((p) => ({ ...p, region: e.target.value }))} placeholder="如：台北" />
                <Field label="所有者" value={form.owner} onChange={(e) => setForm((p) => ({ ...p, owner: e.target.value }))} placeholder="如：供应商A" />
              </div>
              <Field label="备注" value={form.remark} onChange={(e) => setForm((p) => ({ ...p, remark: e.target.value }))} />
            </FieldGroup>
          )}

          <FieldGroup title="SSH 连接">
            <p className="text-sm text-soft">接入脚本会将面板公钥写入此用户的 <code>authorized_keys</code>。</p>
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="SSH 用户" value={form.ssh_user} onChange={(e) => setForm((p) => ({ ...p, ssh_user: e.target.value }))} />
              <Field label="SSH 端口" type="number" value={form.ssh_port} onChange={(e) => setForm((p) => ({ ...p, ssh_port: e.target.value }))} />
            </div>
          </FieldGroup>

          <div>
            <SelectField
              label="接入命令有效期"
              value={form.ttl_seconds}
              onChange={(v) => setForm((p) => ({ ...p, ttl_seconds: v }))}
              options={TTL_OPTIONS}
            />
            <p className="mt-1.5 text-sm text-soft">命令仅可使用一次，首次执行后绑定源 IP。</p>
          </div>

          {err && <p className="text-sm text-[var(--danger)]">{err}</p>}
        </div>
      )}

      {step === 'waiting' && token && (
        <div className="space-y-4">
          <div className="rounded-xl border border-[var(--border)] bg-[var(--panel-muted)] p-4">
            <div className="flex items-center justify-between text-sm">
              <span className="font-semibold">等待节点注册</span>
              <span className="font-mono text-soft">剩余 {remainingLabel}</span>
            </div>
            {token.xray_version && (
              <p className="mt-2 text-sm text-soft">
                安装版本 <code>Xray {token.xray_version}</code>。更改版本后需重新生成命令。
              </p>
            )}
            <p className="mt-2 text-sm leading-6 text-[var(--warning)]">
              <AlertTriangle className="mr-1 inline h-4 w-4" />请在安全组和防火墙放行面板出网 IP
              {token.panel_outbound_ip && <> <code>{token.panel_outbound_ip}</code></>} 对 SSH 端口的访问，否则后续同步会失败。
            </p>
          </div>

          <div className="rounded-xl border border-[var(--border)] bg-[var(--code-bg)] p-4">
            <div className="mb-2 flex flex-wrap items-center justify-between gap-2 text-sm text-soft">
              <span className="inline-flex items-center gap-1.5">
                <Terminal className="h-3.5 w-3.5" />
                在{isReplace ? '新服务器' : '目标服务器'}上以 root 身份执行
              </span>
              <button
                type="button"
                onClick={handleCopyCommand}
                className={`inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium text-white shadow-sm transition ${
                  copied
                    ? 'bg-[var(--success)]'
                    : 'bg-[var(--accent)] hover:brightness-110'
                }`}
              >
                <Copy className="h-3.5 w-3.5" />
                {copied ? '已复制' : '复制命令'}
              </button>
            </div>
            <pre className="overflow-x-auto whitespace-pre-wrap break-all font-mono text-sm text-[var(--code-text)]">
              {token.curl_command}
            </pre>
          </div>

          {token.used_by_ip && (
            <p className="text-sm text-soft">此命令已绑定源 IP <code>{token.used_by_ip}</code>，仅可从该地址执行。</p>
          )}
        </div>
      )}

      {step === 'success' && token && (
        <div className="space-y-3 py-6 text-center">
          <Sparkles className="mx-auto h-10 w-10 text-[var(--success)]" />
          <p className="text-base font-semibold">{isReplace ? '新机器已注册，节点更换完成' : '节点已成功注册'}</p>
          {token.reachable === true && (
            <p className="text-sm text-[var(--success)]">
              面板已连通节点 SSH{typeof token.reachable_latency_ms === 'number' && token.reachable_latency_ms > 0
                ? `（延迟 ${token.reachable_latency_ms} ms）`
                : ''}
              {isReplace && '，已自动触发配置同步'}
            </p>
          )}
          {isReplace && replaceNode?.domain && (
            <p className="text-sm text-[var(--warning)]">
              <AlertTriangle className="mr-1 inline h-3.5 w-3.5 align-[-2px]" />请将域名 <code className="mx-1 rounded bg-[var(--panel-strong)] px-1 font-mono">{replaceNode.domain}</code> 的 DNS 解析指向新 IP
            </p>
          )}
          {token.reachable === false && (
            <div className="mx-auto max-w-md space-y-2 rounded-xl border border-[var(--warning-border)] bg-[var(--warning-soft)] p-3 text-left">
              <p className="text-sm font-semibold text-[var(--warning)]"><AlertTriangle className="mr-1 inline h-4 w-4" />面板暂时无法连接节点 SSH</p>
              <p className="text-sm text-[var(--warning)]">
                {token.reachable_message || 'SSH 端口探针失败，可能是防火墙未放行。'}
              </p>
              <p className="text-sm text-soft">放行后，在节点列表点击「同步」推送配置。</p>
            </div>
          )}
          <p className="text-sm text-soft">即将返回节点列表。</p>
        </div>
      )}

      {step === 'expired' && (
        <div className="space-y-3 py-6 text-center">
          <p className="text-base font-semibold text-[var(--warning)]">接入命令已过期</p>
          <p className="text-sm text-soft">尚未完成注册时，请重新生成命令。已注册的节点会保留。</p>
        </div>
      )}
    </Modal>
  )
}
