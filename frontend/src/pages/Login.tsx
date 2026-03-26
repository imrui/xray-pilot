import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Activity, LockKeyhole, Shield } from 'lucide-react'
import { useAuthStore } from '@/store/auth'
import request from '@/lib/axios'
import type { ApiResponse } from '@/types'

export default function Login() {
  const navigate = useNavigate()
  const setToken = useAuthStore((s) => s.setToken)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await request.post<ApiResponse<{ token: string }>>('/auth/login', {
        username,
        password,
      })
      if (res.data.code === 0 && res.data.data?.token) {
        setToken(res.data.data.token)
        navigate('/')
      } else {
        setError(res.data.message || '登录失败')
      }
    } catch {
      setError('网络错误，请稍后重试')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden px-4">
      <div className="pointer-events-none absolute inset-0">
        <div className="absolute left-[10%] top-[8%] h-52 w-52 rounded-full bg-cyan-400/12 blur-[110px]" />
        <div className="absolute bottom-[8%] right-[8%] h-64 w-64 rounded-full bg-emerald-400/12 blur-[120px]" />
      </div>

      <div className="relative z-10 grid w-full max-w-5xl gap-6 lg:grid-cols-[1.15fr_0.85fr]">
        <div className="panel hidden rounded-[36px] p-8 lg:flex lg:flex-col lg:justify-between">
          <div>
            <div className="inline-flex items-center gap-2 rounded-full border border-[var(--border-strong)] bg-[var(--panel-muted)] px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.2em] text-faint">
              <Activity className="h-3.5 w-3.5 text-[var(--accent)]" />
              Network Operations
            </div>
            <h1 className="mt-6 max-w-md text-4xl font-semibold leading-tight tracking-[-0.06em]">
              用更清晰的界面管理节点、协议和订阅分发。
            </h1>
            <p className="mt-4 max-w-xl text-sm leading-7 text-soft">
              xray-pilot 面向运维控制台而不是展示型落地页。界面重点放在状态密度、操作反馈和低干扰阅读，而不是浮夸装饰。
            </p>
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            <div className="panel-muted rounded-[24px] p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-faint">Security</div>
              <div className="mt-2 text-base font-semibold">统一权限入口</div>
              <p className="mt-2 text-sm leading-6 text-soft">登录后进入集中式控制台，所有敏感操作都保留明确反馈。</p>
            </div>
            <div className="panel-muted rounded-[24px] p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-faint">Signal</div>
              <div className="mt-2 text-base font-semibold">高可读深色模式</div>
              <p className="mt-2 text-sm leading-6 text-soft">配色与层级强调日志、表格和协议配置，不牺牲信息扫描效率。</p>
            </div>
          </div>
        </div>

        <div className="panel-strong rounded-[32px] p-8 md:p-10">
          <div className="mb-8 flex flex-col items-center text-center">
            <div className="mb-5 flex h-14 w-14 items-center justify-center rounded-2xl bg-[var(--accent-soft)] text-[var(--accent)]">
              <Shield className="h-7 w-7" strokeWidth={2.5} />
            </div>
            <h2 className="text-2xl font-semibold tracking-[-0.04em]">xray-pilot</h2>
            <p className="mt-2 text-sm font-medium text-soft">节点管理控制台</p>
          </div>

          <form onSubmit={handleLogin} className="space-y-5">
            <div className="space-y-1.5">
              <label className="text-[12px] font-semibold uppercase tracking-[0.16em] text-faint">用户名</label>
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="w-full rounded-2xl border bg-[var(--panel-muted)] px-4 py-3 text-sm text-[var(--text)] placeholder:text-faint transition-all duration-200 focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-soft)]"
                placeholder="管理员用户名"
                required
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-[12px] font-semibold uppercase tracking-[0.16em] text-faint">密码</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full rounded-2xl border bg-[var(--panel-muted)] px-4 py-3 text-sm text-[var(--text)] placeholder:text-faint transition-all duration-200 focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-soft)]"
                placeholder="输入登录密码"
                required
              />
            </div>
            {error && <p className="text-center text-[12px] font-medium text-red-500">{error}</p>}
            <button
              type="submit"
              disabled={loading}
              className="mt-2 flex w-full items-center justify-center gap-2 rounded-2xl bg-[var(--accent)] px-4 py-3 text-sm font-semibold text-slate-950 transition-all duration-200 hover:brightness-105 disabled:opacity-50"
            >
              {loading ? <span className="h-4 w-4 animate-spin rounded-full border-2 border-slate-950/25 border-t-slate-950" /> : <LockKeyhole className="h-4 w-4" />}
              {loading ? '登录中…' : '登录系统'}
            </button>
          </form>
        </div>
      </div>
    </div>
  )
}
