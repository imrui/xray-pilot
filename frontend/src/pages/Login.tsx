import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowRight, LockKeyhole, Shield, Sparkles } from 'lucide-react'
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
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-[var(--app-bg)] px-4 py-10">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_top_left,rgba(21,143,118,0.08),transparent_22%),radial-gradient(circle_at_bottom_right,rgba(24,33,47,0.07),transparent_28%)]" />

      <div className="relative z-10 grid w-full max-w-6xl gap-6 lg:grid-cols-[1.12fr_0.88fr]">
        <div className="hidden rounded-[28px] border border-[var(--border)] bg-[var(--panel-strong)] p-8 shadow-[var(--shadow-panel)] lg:flex lg:flex-col lg:justify-between">
          <div>
            <div className="inline-flex items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--panel-muted)] px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">
              <Sparkles className="h-3.5 w-3.5 text-[var(--accent)]" />
              Xray Operations
            </div>
            <h1 className="mt-6 max-w-lg text-4xl font-semibold leading-tight tracking-[-0.06em]">
              更清晰地管理节点、协议、用户和分发链路。
            </h1>
            <p className="mt-4 max-w-xl text-sm leading-7 text-soft">
              这一版登录入口延续 v0 的控制台方向，减少装饰性噪声，让后续进入列表、抽屉和诊断页面时层级更统一。
            </p>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="rounded-[22px] border border-[var(--border)] bg-[var(--panel-muted)] p-5">
              <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">Overview</div>
              <div className="mt-2 text-base font-semibold">集中式后台入口</div>
              <p className="mt-2 text-sm leading-6 text-soft">登录后直接进入统一控制台，侧边导航、顶部工具栏与高频表格操作保持一致。</p>
            </div>
            <div className="rounded-[22px] border border-[var(--border)] bg-[var(--panel-muted)] p-5">
              <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">Workflow</div>
              <div className="mt-2 text-base font-semibold">为运维流程设计</div>
              <p className="mt-2 text-sm leading-6 text-soft">界面优先照顾节点同步、协议配置、订阅分发和部署诊断这些真实工作流。</p>
            </div>
          </div>
        </div>

        <div className="rounded-[28px] border border-[var(--border)] bg-[var(--panel-strong)] p-8 shadow-[var(--shadow-panel)] md:p-10">
          <div className="mb-8 flex flex-col items-center text-center">
            <div className="mb-5 flex h-14 w-14 items-center justify-center rounded-2xl bg-[var(--accent-soft)] text-[var(--accent)]">
              <Shield className="h-7 w-7" strokeWidth={2.5} />
            </div>
            <h2 className="text-2xl font-semibold tracking-[-0.04em]">xray-pilot</h2>
            <p className="mt-2 text-sm font-medium text-soft">Infrastructure control console</p>
          </div>

          <form onSubmit={handleLogin} className="space-y-5">
            <div className="space-y-1.5">
              <label className="text-[12px] font-semibold uppercase tracking-[0.16em] text-faint">用户名</label>
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="h-12 w-full rounded-xl border border-[var(--border)] bg-[var(--panel-muted)] px-4 text-sm text-[var(--text)] placeholder:text-faint transition-all duration-200 focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
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
                className="h-12 w-full rounded-xl border border-[var(--border)] bg-[var(--panel-muted)] px-4 text-sm text-[var(--text)] placeholder:text-faint transition-all duration-200 focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
                placeholder="输入登录密码"
                required
              />
            </div>
            {error && <p className="text-center text-[12px] font-medium text-red-500">{error}</p>}
            <button
              type="submit"
              disabled={loading}
              className="mt-2 flex h-12 w-full items-center justify-center gap-2 rounded-xl bg-[var(--accent)] px-4 text-sm font-semibold text-[var(--accent-contrast)] transition-all duration-200 hover:bg-[var(--accent-strong)] disabled:opacity-50"
            >
              {loading ? (
                <span className="h-4 w-4 animate-spin rounded-full border-2 border-[var(--accent-contrast)]/30 border-t-[var(--accent-contrast)]" />
              ) : (
                <>
                  <LockKeyhole className="h-4 w-4" />
                  <ArrowRight className="h-4 w-4" />
                </>
              )}
              {loading ? '登录中…' : '登录系统'}
            </button>
          </form>
        </div>
      </div>
    </div>
  )
}
