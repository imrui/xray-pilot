import { useState } from 'react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  Activity,
  ChevronRight,
  Layers,
  LogOut,
  Menu,
  Moon,
  ScrollText,
  Server,
  Settings,
  Shield,
  Sun,
  Users,
  X,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/store/auth'
import { useThemeStore } from '@/store/theme'

const navItems = [
  { path: '/users', label: '用户', icon: Users },
  { path: '/groups', label: '分组', icon: Layers },
  { path: '/nodes', label: '节点', icon: Server },
  { path: '/profiles', label: '协议', icon: Shield },
  { path: '/logs', label: '日志', icon: ScrollText },
  { path: '/settings', label: '系统', icon: Settings },
]

export default function Layout() {
  const location = useLocation()
  const navigate = useNavigate()
  const logout = useAuthStore((s) => s.logout)
  const { theme, toggleTheme } = useThemeStore()
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  const currentView = navItems.find((item) => location.pathname.startsWith(item.path))?.label ?? '控制台'

  return (
    <div className="relative min-h-screen overflow-hidden text-[var(--text)]">
      <div className="pointer-events-none absolute inset-0">
        <div className="absolute left-[-8%] top-[-6%] h-72 w-72 rounded-full bg-teal-400/10 blur-[110px]" />
        <div className="absolute bottom-[-10%] right-[-6%] h-80 w-80 rounded-full bg-orange-300/8 blur-[120px]" />
      </div>

      <header className="panel-strong fixed inset-x-3 top-3 z-50 flex h-16 items-center justify-between rounded-2xl px-4 lg:hidden">
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-[var(--accent-soft)] text-[var(--accent)]">
            <Shield className="h-4 w-4" />
          </div>
          <div>
            <div className="text-sm font-semibold tracking-[-0.03em]">xray-pilot</div>
            <div className="text-[11px] uppercase tracking-[0.22em] text-faint">Network Control</div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={toggleTheme} className="rounded-xl p-2 text-soft transition hover:bg-[var(--panel-muted)]">
            {theme === 'dark' ? <Sun className="h-5 w-5" /> : <Moon className="h-5 w-5" />}
          </button>
          <button onClick={() => setMobileMenuOpen(!mobileMenuOpen)} className="rounded-xl p-2 transition hover:bg-[var(--panel-muted)]">
            {mobileMenuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
          </button>
        </div>
      </header>

      {mobileMenuOpen && (
        <div className="fixed inset-0 z-40 lg:hidden">
          <div className="absolute inset-0 bg-slate-950/48 backdrop-blur-sm" onClick={() => setMobileMenuOpen(false)} />
          <div className="panel-strong absolute inset-x-3 bottom-3 top-20 overflow-auto rounded-[28px] p-4">
            <div className="mb-4 flex items-center justify-between border-b border-[var(--border)] pb-4">
              <div>
                <div className="text-sm font-semibold">Navigation</div>
                <div className="mt-1 text-xs text-faint">Quick access to core operations</div>
              </div>
              <Activity className="h-4 w-4 text-[var(--accent)]" />
            </div>
            <nav className="space-y-2">
              {navItems.map((item) => {
                const active = location.pathname.startsWith(item.path)
                return (
                  <Link
                    key={item.path}
                    to={item.path}
                    onClick={() => setMobileMenuOpen(false)}
                    className={cn(
                      'flex items-center justify-between rounded-2xl px-4 py-3 text-sm font-semibold transition-all',
                      active
                        ? 'bg-[var(--accent-soft)] text-[var(--accent)] ring-1 ring-[var(--border-strong)]'
                        : 'text-soft hover:bg-[var(--panel-muted)] hover:text-[var(--text)]'
                    )}
                  >
                    <span className="flex items-center gap-3">
                      <item.icon className="h-5 w-5" />
                      {item.label}
                    </span>
                    <ChevronRight className="h-4 w-4 opacity-50" />
                  </Link>
                )
              })}
            </nav>
            <div className="mt-4 border-t border-[var(--border)] pt-4">
              <button
                onClick={handleLogout}
                className="flex w-full items-center justify-center gap-2 rounded-2xl bg-rose-500/10 py-3 text-sm font-semibold text-rose-500 transition hover:bg-rose-500/16"
              >
                <LogOut className="h-4 w-4" />
                退出登录
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="relative z-10 flex min-h-screen gap-4 p-3 pt-20 lg:pt-3">
        <aside className="hidden w-[280px] shrink-0 lg:block">
          <div className="panel flex h-[calc(100vh-24px)] flex-col rounded-[32px] p-4">
            <div className="flex items-center gap-3 rounded-[24px] border border-[var(--border)] bg-[var(--panel-muted)] p-4">
              <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-[var(--accent-soft)] text-[var(--accent)]">
                <Shield className="h-5 w-5" strokeWidth={2.4} />
              </div>
              <div>
                <h1 className="text-base font-semibold tracking-[-0.04em]">xray-pilot</h1>
                <p className="mt-1 text-[11px] uppercase tracking-[0.22em] text-faint">Infrastructure Console</p>
              </div>
            </div>

            <div className="mt-5 rounded-[24px] border border-[var(--border)] bg-[var(--panel-muted)] p-4">
              <div className="flex items-center justify-between">
                <div>
                  <div className="text-[11px] uppercase tracking-[0.22em] text-faint">Status</div>
                  <div className="mt-2 text-sm font-semibold">Control plane online</div>
                </div>
                <div className="h-2.5 w-2.5 rounded-full bg-[var(--accent)] shadow-[0_0_16px_var(--accent)]" />
              </div>
              <div className="mt-3 text-xs leading-5 text-soft">
                用户、节点、协议和系统参数集中管理，强调状态密度、响应反馈和清晰操作路径。
              </div>
            </div>

            <div className="mt-5 text-[11px] font-semibold uppercase tracking-[0.22em] text-faint">Navigation</div>
            <nav className="mt-3 space-y-1.5">
              {navItems.map((item) => {
                const active = location.pathname.startsWith(item.path)
                return (
                  <Link
                    key={item.path}
                    to={item.path}
                    className={cn(
                      'group flex items-center gap-3 rounded-2xl px-4 py-3 text-sm font-semibold transition-all',
                      active
                        ? 'bg-[var(--accent-soft)] text-[var(--accent)] ring-1 ring-[var(--border-strong)]'
                        : 'text-soft hover:bg-[var(--panel-muted)] hover:text-[var(--text)]'
                    )}
                  >
                    <item.icon className={cn('h-[18px] w-[18px] transition-transform group-hover:scale-110', active && 'scale-110')} />
                    <span className="flex-1">{item.label}</span>
                    <ChevronRight className={cn('h-4 w-4 transition', active ? 'opacity-100' : 'opacity-0 group-hover:opacity-60')} />
                  </Link>
                )
              })}
            </nav>
          </div>
        </aside>

        <main className="min-w-0 flex-1">
          <div className="panel relative min-h-[calc(100vh-24px)] rounded-[32px] p-4 md:p-6">
            <div className="mb-4 hidden items-center justify-end gap-3 lg:flex">
              <div className="rounded-full border border-[var(--border)] bg-[var(--panel-muted)] px-4 py-2 text-xs font-semibold uppercase tracking-[0.2em] text-faint">
                {currentView}
              </div>
              <div className="rounded-[18px] border border-[var(--border)] bg-[var(--panel-muted)] p-1">
                <div className="grid grid-cols-2 gap-1">
                  <button
                    onClick={() => {
                      if (theme === 'dark') toggleTheme()
                    }}
                    className={cn(
                      'flex items-center justify-center gap-2 rounded-2xl px-3 py-2 text-sm font-medium transition',
                      theme === 'light' ? 'bg-[var(--panel-strong)] text-[var(--text)] shadow-sm' : 'text-soft hover:bg-[var(--panel)]'
                    )}
                  >
                    <Sun className="h-4 w-4" />
                    Light
                  </button>
                  <button
                    onClick={() => {
                      if (theme === 'light') toggleTheme()
                    }}
                    className={cn(
                      'flex items-center justify-center gap-2 rounded-2xl px-3 py-2 text-sm font-medium transition',
                      theme === 'dark' ? 'bg-[var(--panel-strong)] text-[var(--text)] shadow-sm' : 'text-soft hover:bg-[var(--panel)]'
                    )}
                  >
                    <Moon className="h-4 w-4" />
                    Dark
                  </button>
                </div>
              </div>
              <button
                onClick={handleLogout}
                className="flex items-center justify-center gap-2 rounded-2xl border border-[var(--danger)]/18 bg-[var(--danger-soft)] px-4 py-3 text-sm font-semibold text-[var(--danger)] transition hover:brightness-105"
              >
                <LogOut className="h-4 w-4" />
                退出登录
              </button>
            </div>

            <div className="min-h-[calc(100vh-88px)] overflow-auto rounded-[26px]">
              <Outlet />
            </div>
          </div>
        </main>
      </div>
    </div>
  )
}
