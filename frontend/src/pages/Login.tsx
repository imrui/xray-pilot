import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowRight, LockKeyhole, Moon, Sparkles, Sun } from 'lucide-react'
import { useAuthStore } from '@/store/auth'
import { useThemeStore } from '@/store/theme'
import request from '@/lib/axios'
import type { ApiResponse } from '@/types'
import { GitHubMark } from '@/components/icons/GitHubMark'
import { Logo } from '@/components/icons/Logo'
import { APP_VERSION } from '@/lib/version'

export default function Login() {
  const navigate = useNavigate()
  const setToken = useAuthStore((s) => s.setToken)
  const { theme, toggleTheme } = useThemeStore()
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
      <button
        onClick={toggleTheme}
        className="absolute right-6 top-6 z-20 inline-flex h-9 w-9 items-center justify-center rounded-md border border-[var(--border)] bg-[var(--panel)] text-soft transition hover:text-[var(--text)]"
        title="切换主题"
      >
        {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
      </button>

      <div className="relative z-10 grid w-full max-w-6xl gap-6 lg:grid-cols-[1.12fr_0.88fr]">
        <div className="hidden rounded-[28px] border border-[var(--border)] bg-[var(--panel-strong)] p-8 shadow-[var(--shadow-panel)] lg:flex lg:flex-col">
          <div>
            <div className="inline-flex items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--panel-muted)] px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">
              <Sparkles className="h-3.5 w-3.5 text-[var(--accent)]" />
              Secure Access
            </div>
            <h1 className="mt-6 max-w-lg text-4xl font-semibold leading-tight tracking-[-0.06em]">
              掌控每一个代理节点
            </h1>
            <p className="mt-4 max-w-xl text-sm leading-7 text-soft">
              Xray多节点统一管理，配置同步，订阅一键分发。
            </p>
          </div>
          <div className="mt-6 grid gap-4 md:grid-cols-3">
            <div className="rounded-[22px] border border-[var(--border)] bg-[var(--panel-muted)] p-5">
              <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">Security</div>
              <div className="mt-2 text-base font-semibold">集中权限控制</div>
              <p className="mt-2 text-sm leading-6 text-soft">统一后台入口，告别散落配置与误操作风险。</p>
            </div>
            <div className="rounded-[22px] border border-[var(--border)] bg-[var(--panel-muted)] p-5">
              <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">Observability</div>
              <div className="mt-2 text-base font-semibold">实时健康监控</div>
              <p className="mt-2 text-sm leading-6 text-soft">节点延迟、同步状态、配置漂移，一览无余。</p>
            </div>
            <div className="rounded-[22px] border border-[var(--border)] bg-[var(--panel-muted)] p-5">
              <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">Automation</div>
              <div className="mt-2 text-base font-semibold">配置自动同步</div>
              <p className="mt-2 text-sm leading-6 text-soft">用户变更即时推送，订阅链接无需手动维护。</p>
            </div>
          </div>
          <div className="mt-8 flex items-center justify-between border-t border-[var(--border)] pt-4 text-xs text-soft">
            <span>© 2026 Xray Pilot. All rights reserved. {APP_VERSION}</span>
            <a
              href="https://github.com/imrui/xray-pilot"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1.5 transition hover:text-[var(--text)]"
            >
              <GitHubMark className="h-3.5 w-3.5" />
              GitHub
            </a>
          </div>
        </div>

        <div className="rounded-[28px] border border-[var(--border)] bg-[var(--panel-strong)] p-8 shadow-[var(--shadow-panel)] md:p-10">
          <div className="mb-8 flex flex-col items-center text-center">
            <div className="mb-5 flex h-14 w-14 items-center justify-center rounded-2xl bg-[var(--accent-soft)] text-[var(--accent)]">
              <Logo size={46} className="text-[var(--accent)]" />
            </div>
            <h2 className="text-2xl font-semibold tracking-[-0.04em]">Xray Pilot</h2>
            <p className="mt-2 text-sm font-medium text-soft">节点管理控制台</p>
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
          <div className="mt-6 flex items-center justify-between border-t border-[var(--border)] pt-4 text-xs text-soft lg:hidden">
            <span>© 2026 Xray Pilot. All rights reserved. {APP_VERSION}</span>
            <a
              href="https://github.com/imrui/xray-pilot"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1.5 transition hover:text-[var(--text)]"
            >
              <GitHubMark className="h-3.5 w-3.5" />
              GitHub
            </a>
          </div>
        </div>
      </div>
    </div>
  )
}
