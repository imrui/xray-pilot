import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Copy, Sparkles, Terminal } from 'lucide-react'
import { installApi, type InstallToken } from '@/lib/api'
import { copyText } from '@/lib/clipboard'
import { Modal } from '@/components/ui/Modal'
import { Field, Btn, FieldGroup, SelectField } from '@/components/ui/Form'
import { pushToast } from '@/lib/notify'

type Step = 'form' | 'waiting' | 'success' | 'expired'

interface Props {
  open: boolean
  onClose: () => void
  // 节点注册成功后由父组件刷新节点列表
  onRegistered?: (nodeId: number) => void
}

const TTL_OPTIONS = [
  { value: '600', label: '10 分钟（默认）' },
  { value: '3600', label: '1 小时' },
  { value: '86400', label: '24 小时' },
]

// OneClickInstallDialog 节点一键接入对话框
// 流程：填表单 → 后端生成 token → 显示 curl 命令带轮询 → 节点上报回 panel 成功 → 关闭
export function OneClickInstallDialog({ open, onClose, onRegistered }: Props) {
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
  const [err, setErr] = useState('')

  // 重置：每次打开对话框回到表单
  useEffect(() => {
    if (open) {
      setStep('form')
      setToken(null)
      setCopied(false)
      setErr('')
      successFiredRef.current = false
    }
  }, [open])

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
          setStep('success')
          // 3 秒后通知父级
          window.setTimeout(() => {
            onRegistered?.(latest.node_id!)
            onClose()
          }, 3000)
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
    if (!form.name.trim()) {
      setErr('节点名不能为空')
      return
    }
    setSubmitting(true)
    try {
      const res = await installApi.create({
        name: form.name.trim(),
        region: form.region.trim() || undefined,
        owner: form.owner.trim() || undefined,
        remark: form.remark.trim() || undefined,
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
      title="一键接入节点"
      size="lg"
      footer={
        step === 'form' ? (
          <>
            <Btn variant="secondary" onClick={handleCancel}>取消</Btn>
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
      {step === 'form' && (
        <div className="space-y-4">
          <p className="text-xs text-soft">
            填写节点元数据，后端生成一次性 token（默认 10 分钟、绑定首次访问 IP），返回完整 curl 命令供你在目标机器上执行。
            脚本自动拉取 panel 公钥、装 xray、回填节点信息。
          </p>
          <FieldGroup title="节点元数据" description="脚本注册时用这里的信息创建节点记录。">
            <Field label="节点名 *" value={form.name} onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))} placeholder="如：tw03-lk" />
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="地区" value={form.region} onChange={(e) => setForm((p) => ({ ...p, region: e.target.value }))} placeholder="如：台北" />
              <Field label="所有者" value={form.owner} onChange={(e) => setForm((p) => ({ ...p, owner: e.target.value }))} placeholder="如：供应商A" />
            </div>
            <Field label="备注" value={form.remark} onChange={(e) => setForm((p) => ({ ...p, remark: e.target.value }))} />
          </FieldGroup>

          <FieldGroup title="SSH 参数" description="脚本会把 panel 自身的 SSH 公钥写入该用户的 authorized_keys。">
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

          {err && <p className="text-sm text-rose-500">{err}</p>}
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
              请以 root 用户执行下面这行命令（若当前为普通用户，先执行 <code className="rounded bg-[var(--panel-strong)] px-1 font-mono">sudo su -</code> 切换）。脚本完成自检后会自动注册回 panel，该对话框会自动关闭并刷新节点列表。
            </p>
          </div>

          <div className="rounded-xl border border-[var(--border)] bg-slate-950 p-4">
            <div className="mb-2 flex items-center justify-between text-xs text-soft">
              <span className="inline-flex items-center gap-1.5">
                <Terminal className="h-3.5 w-3.5" />
                目标机器执行
              </span>
              <button
                type="button"
                onClick={handleCopyCommand}
                className="inline-flex items-center gap-1.5 rounded-md border border-white/10 px-2 py-1 text-[11px] text-[var(--accent)] transition hover:bg-white/5"
              >
                <Copy className="h-3 w-3" />
                {copied ? '已复制' : '复制命令'}
              </button>
            </div>
            <pre className="overflow-x-auto whitespace-pre-wrap break-all font-mono text-xs text-slate-100">
              {token.curl_command}
            </pre>
          </div>

          {token.used_by_ip && (
            <p className="text-xs text-soft">已绑定源 IP：<span className="font-mono">{token.used_by_ip}</span>（异地执行将被拒绝）</p>
          )}
        </div>
      )}

      {step === 'success' && (
        <div className="space-y-3 py-6 text-center">
          <Sparkles className="mx-auto h-10 w-10 text-emerald-500" />
          <p className="text-base font-semibold">节点已成功注册</p>
          <p className="text-xs text-soft">正在刷新节点列表，3 秒后自动关闭…</p>
        </div>
      )}

      {step === 'expired' && (
        <div className="space-y-3 py-6 text-center">
          <p className="text-base font-semibold text-amber-500">Token 已过期</p>
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
