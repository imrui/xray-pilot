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
        ) : step === 'waiting' ? (
          <>
            <Btn variant="secondary" onClick={handleCancel}>关闭</Btn>
            <span className="text-xs text-soft">在目标机器上执行命令，注册回 panel 后自动关闭</span>
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
          <p className="text-xs text-soft">
            当前节点 IP <code className="rounded bg-[var(--panel-strong)] px-1 font-mono">{replaceNode.ip}</code>
            {replaceNode.domain && <>，域名 <code className="rounded bg-[var(--panel-strong)] px-1 font-mono">{replaceNode.domain}</code></>}。
            两种更换方式均保留节点的分组、协议密钥与端口/SNI 覆盖，客户端订阅无需更换。
          </p>
          <button
            type="button"
            onClick={() => { setStep('ip'); setErr('') }}
            className="flex w-full items-start gap-3 rounded-xl border border-[var(--border)] bg-[var(--panel-muted)] p-4 text-left transition hover:border-[var(--accent)]"
          >
            <PencilLine className="mt-0.5 h-5 w-5 shrink-0 text-[var(--accent)]" />
            <span>
              <span className="block text-sm font-semibold">仅更换 IP</span>
              <span className="mt-1 block text-xs text-soft">
                服务器本身没换（如迁移弹性 IP）。直接改节点 IP 并标记待同步{replaceNode.domain ? '，域名解析需手动改 DNS' : ''}。
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
              <span className="mt-1 block text-xs text-soft">
                换了一台新机器。生成一次性命令在新机执行：装 xray、写入 panel 公钥、注册回本节点并自动同步配置。
              </span>
            </span>
          </button>
        </div>
      )}

      {step === 'ip' && replaceNode && (
        <div className="space-y-4">
          <Field label="当前 IP" value={replaceNode.ip} disabled readOnly />
          <Field label="新 IP *" value={newIP} onChange={(e) => setNewIP(e.target.value)} placeholder="如：34.21.157.200" />
          {replaceNode.domain && (
            <p className="text-xs text-[var(--warning)]">
              <AlertTriangle className="mr-1 inline h-3.5 w-3.5 align-[-2px]" />该节点配置了域名 <code className="mx-1 rounded bg-[var(--panel-strong)] px-1 font-mono">{replaceNode.domain}</code>，
              保存后请手动将 DNS 解析指向新 IP，客户端订阅才能正常连接。
            </p>
          )}
          <p className="text-xs text-soft">保存后节点进入待同步状态，请在节点列表点「同步」推送配置到新 IP。</p>
          {err && <p className="text-sm text-[var(--danger)]">{err}</p>}
        </div>
      )}

      {step === 'form' && (
        <div className="space-y-4">
          {isReplace ? (
            <p className="text-xs text-soft">
              在<span className="font-semibold">新机器</span>上执行生成的命令：脚本自动装 xray、写入 panel 公钥、注册回节点
              「{replaceNode!.name}」。注册成功后自动推送原有配置（Reality 密钥不变），节点 ID 与订阅均不变。
            </p>
          ) : (
            <p className="text-xs text-soft">
              填写节点元数据，后端生成一次性 token（默认 10 分钟、绑定首次访问 IP），返回完整 curl 命令供你在目标机器上执行。
              脚本自动拉取 panel 公钥、装 xray、回填节点信息。
            </p>
          )}
          {!isReplace && (
            <FieldGroup title="节点元数据" description="脚本注册时用这里的信息创建节点记录。">
              <Field label="节点名 *" value={form.name} onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))} placeholder="如：tw03-lk" />
              <div className="grid gap-4 md:grid-cols-2">
                <Field label="地区" value={form.region} onChange={(e) => setForm((p) => ({ ...p, region: e.target.value }))} placeholder="如：台北" />
                <Field label="所有者" value={form.owner} onChange={(e) => setForm((p) => ({ ...p, owner: e.target.value }))} placeholder="如：供应商A" />
              </div>
              <Field label="备注" value={form.remark} onChange={(e) => setForm((p) => ({ ...p, remark: e.target.value }))} />
            </FieldGroup>
          )}

          <FieldGroup title="SSH 参数" description={isReplace ? '新机器的 SSH 用户与端口；脚本会把 panel 公钥写入该用户的 authorized_keys。' : '脚本会把 panel 自身的 SSH 公钥写入该用户的 authorized_keys。'}>
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="SSH 用户" value={form.ssh_user} onChange={(e) => setForm((p) => ({ ...p, ssh_user: e.target.value }))} />
              <Field label="SSH 端口" type="number" value={form.ssh_port} onChange={(e) => setForm((p) => ({ ...p, ssh_port: e.target.value }))} />
            </div>
          </FieldGroup>

          <FieldGroup title="Token 有效期">
            <SelectField
              label="过期时长"
              value={form.ttl_seconds}
              onChange={(v) => setForm((p) => ({ ...p, ttl_seconds: v }))}
              options={TTL_OPTIONS}
            />
          </FieldGroup>

          {err && <p className="text-sm text-[var(--danger)]">{err}</p>}
        </div>
      )}

      {step === 'waiting' && token && (
        <div className="space-y-4">
          <div className="rounded-xl border border-[var(--border)] bg-[var(--panel-muted)] p-4">
            <div className="flex items-center justify-between text-sm">
              <span className="font-semibold">等待节点执行中…</span>
              <span className="font-mono text-soft">剩余 {remainingLabel}</span>
            </div>
            <p className="mt-2 text-xs text-soft">
              请在{isReplace ? '新机器' : '目标机器'}上以 root 用户执行下面这行命令（若当前为普通用户，先执行 <code className="rounded bg-[var(--panel-strong)] px-1 font-mono">sudo su -</code> 切换）。脚本完成自检后会自动注册回 panel，该对话框会自动关闭并刷新节点列表。
            </p>
            <p className="mt-2 text-xs text-[var(--warning)]">
              ⚠️ 同时确保节点防火墙允许面板{token.panel_outbound_ip ? (
                <> IP <code className="mx-1 rounded bg-[var(--panel-strong)] px-1 font-mono text-[var(--warning)]">{token.panel_outbound_ip}</code></>
              ) : ' 出网 IP'}通过 SSH 端口访问（云厂商安全组 / ufw / firewalld / iptables 任一层拦截都会让后续同步失败）
            </p>
          </div>

          <div className="rounded-xl border border-[var(--border)] bg-[var(--code-bg)] p-4">
            <div className="mb-2 flex items-center justify-between text-xs text-soft">
              <span className="inline-flex items-center gap-1.5">
                <Terminal className="h-3.5 w-3.5" />
                目标机器执行
              </span>
              <button
                type="button"
                onClick={handleCopyCommand}
                className={`inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium text-white shadow-sm transition ${
                  copied
                    ? 'bg-[var(--success)]'
                    : 'bg-[var(--accent)] hover:brightness-110'
                }`}
              >
                <Copy className="h-3.5 w-3.5" />
                {copied ? '已复制' : '复制命令'}
              </button>
            </div>
            <pre className="overflow-x-auto whitespace-pre-wrap break-all font-mono text-xs text-[var(--code-text)]">
              {token.curl_command}
            </pre>
          </div>

          {token.used_by_ip && (
            <p className="text-xs text-soft">已绑定源 IP：<span className="font-mono">{token.used_by_ip}</span>（异地执行将被拒绝）</p>
          )}
        </div>
      )}

      {step === 'success' && token && (
        <div className="space-y-3 py-6 text-center">
          <Sparkles className="mx-auto h-10 w-10 text-[var(--success)]" />
          <p className="text-base font-semibold">{isReplace ? '新机器已注册，节点更换完成' : '节点已成功注册'}</p>
          {token.reachable === true && (
            <p className="text-xs text-[var(--success)]">
              ✅ panel 已可正常 SSH 到节点{typeof token.reachable_latency_ms === 'number' && token.reachable_latency_ms > 0
                ? `（延迟 ${token.reachable_latency_ms} ms）`
                : ''}
              {isReplace && '，已自动触发配置同步'}
            </p>
          )}
          {isReplace && replaceNode?.domain && (
            <p className="text-xs text-[var(--warning)]">
              <AlertTriangle className="mr-1 inline h-3.5 w-3.5 align-[-2px]" />请将域名 <code className="mx-1 rounded bg-[var(--panel-strong)] px-1 font-mono">{replaceNode.domain}</code> 的 DNS 解析指向新 IP
            </p>
          )}
          {token.reachable === false && (
            <div className="mx-auto max-w-md space-y-2 rounded-xl border border-[var(--warning-border)] bg-[var(--warning-soft)] p-3 text-left">
              <p className="text-xs font-semibold text-[var(--warning)]"><AlertTriangle className="mr-1 inline h-3.5 w-3.5 align-[-2px]" />panel 暂时无法 SSH 到节点</p>
              <p className="text-xs text-[var(--warning)]">
                {token.reachable_message || 'SSH 端口探针失败，可能是防火墙未放行。'}
              </p>
              <p className="text-[11px] text-soft">放行后回节点列表点「同步」即可使配置生效。</p>
            </div>
          )}
          <p className="text-xs text-soft">正在刷新节点列表，自动关闭…</p>
        </div>
      )}

      {step === 'expired' && (
        <div className="space-y-3 py-6 text-center">
          <p className="text-base font-semibold text-[var(--warning)]">Token 已过期</p>
          <p className="text-xs text-soft">如果脚本尚未执行完毕，请重新生成 token；已经执行成功的节点会被保留。</p>
        </div>
      )}
    </Modal>
  )
}

// PrecheckHook 由父组件调用以在打开对话框前确认 panel 已配 SSH 密钥。
// 调用 install create 时后端会再校验一次；这里只做提前提示，避免管理员填完表单才发现。
export function notifyPanelSSHMissing(message: string) {
  pushToast({
    title: '一键接入不可用',
    description: message,
    variant: 'warning',
  })
}
