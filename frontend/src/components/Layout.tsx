import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import {
  ChevronLeft,
  ChevronRight,
  LayoutDashboard,
  Layers,
  LogOut,
  Menu,
  Moon,
  ScrollText,
  Search,
  Server,
  Settings,
  Shield,
  Sun,
  UserRound,
  Users,
  X,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/store/auth'
import { useThemeStore } from '@/store/theme'
import { GitHubMark } from '@/components/icons/GitHubMark'
import { Logo } from '@/components/icons/Logo'
import { APP_VERSION } from '@/lib/version'
import { systemApi } from '@/lib/api'
import { SyncReminderBanner } from '@/components/SyncReminderBanner'

const navGroups = [
  {
    title: '概览',
    items: [{ path: '/dashboard', label: '仪表总览', subtitle: '系统概览与快捷操作', icon: LayoutDashboard }],
  },
  {
    title: '用户订阅',
    items: [{ path: '/users', label: '用户管理', subtitle: '管理用户与订阅权限', icon: Users }],
  },
  {
    title: '资源管理',
    items: [
      { path: '/nodes', label: '节点管理', subtitle: '管理服务节点与配置同步', icon: Server },
      { path: '/profiles', label: '协议配置', subtitle: '维护协议模板与节点密钥', icon: Shield },
      { path: '/groups', label: '分组管理', subtitle: '管理节点分组和策略映射', icon: Layers },
    ],
  },
  {
    title: '运维工具',
    items: [
      { path: '/logs', label: '操作日志', subtitle: '查看系统行为与同步记录', icon: ScrollText },
      { path: '/settings', label: '系统设置', subtitle: '配置系统参数与诊断项', icon: Settings },
    ],
  },
]

const allNavItems = navGroups.flatMap((group) => group.items)

export default function Layout() {
  const location = useLocation()
  const navigate = useNavigate()
  const logout = useAuthStore((s) => s.logout)
  const { theme, toggleTheme } = useThemeStore()
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const { data: syncSummary } = useQuery({
    queryKey: ['sync-summary'],
    queryFn: () => systemApi.getSyncSummary().then((r) => r.data.data!),
    refetchInterval: 30000,
    refetchOnWindowFocus: true,
    retry: 1,
  })

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  const currentView = useMemo(
    () => allNavItems.find((item) => location.pathname.startsWith(item.path))?.label ?? '控制台',
    [location.pathname]
  )
  const currentSubTitle = useMemo(
    () => allNavItems.find((item) => location.pathname.startsWith(item.path))?.subtitle ?? '可视化管理与系统配置',
    [location.pathname]
  )

  const sidebarWidth = collapsed ? 'w-[74px]' : 'w-[240px]'

  return (
    <div className="min-h-screen bg-[var(--app-bg)] text-[var(--text)] lg:h-screen lg:overflow-hidden">
      <header className="sticky top-0 z-50 border-b border-[var(--border)] bg-[color:var(--panel-strong)]/96 px-4 py-3 backdrop-blur-xl lg:hidden">
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 items-center justify-center text-[var(--accent)]">
            <Logo size={32} className="text-[var(--accent)]" />
          </div>
          {!collapsed && (
            <div>
              <div className="text-sm font-semibold tracking-[-0.03em]">Xray Pilot</div>
              <div className="text-[11px] uppercase tracking-[0.14em] text-faint">Console</div>
            </div>
          )}
          <div className="ml-auto flex items-center gap-2">
            <button
              onClick={toggleTheme}
              className="inline-flex h-9 w-9 items-center justify-center rounded-md border border-[var(--border)] bg-[var(--panel)] text-soft transition hover:text-[var(--text)]"
              title="切换主题"
            >
              {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </button>
            <button
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
              className="rounded-md border border-[var(--border)] bg-[var(--panel)] p-2 transition hover:text-[var(--text)]"
            >
              {mobileMenuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
            </button>
          </div>
        </div>
      </header>

      {mobileMenuOpen && (
        <div className="fixed inset-0 z-40 lg:hidden">
          <div className="absolute inset-0 bg-slate-950/42 backdrop-blur-sm" onClick={() => setMobileMenuOpen(false)} />
          <div className="absolute inset-x-3 bottom-3 top-20 overflow-auto rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] p-4 shadow-[var(--shadow-panel)]">
            <div className="mb-4 flex items-center justify-between border-b border-[var(--border)] pb-4">
              <div>
                <div className="text-sm font-semibold">导航菜单</div>
                <div className="mt-1 text-xs text-faint">当前页面：{currentView}</div>
              </div>
            </div>
            <div className="space-y-5">
              {navGroups.map((group) => (
                <div key={group.title}>
                  <div className="mb-2 text-[11px] font-medium uppercase tracking-[0.14em] text-faint">{group.title}</div>
                  <nav className="space-y-1">
                    {group.items.map((item) => {
                      const active = location.pathname.startsWith(item.path)
                      return (
                        <Link
                          key={item.path}
                          to={item.path}
                          onClick={() => setMobileMenuOpen(false)}
                          className={cn(
                            'flex items-center justify-between rounded-md px-3 py-2.5 text-sm font-medium transition-all',
                            active
                              ? 'bg-[var(--accent-soft)] text-[var(--accent)]'
                              : 'text-soft hover:bg-[var(--panel-muted)] hover:text-[var(--text)]'
                          )}
                        >
                          <span className="flex items-center gap-3">
                            <item.icon className="h-4 w-4" />
                            {item.label}
                          </span>
                          <ChevronRight className="h-4 w-4 opacity-50" />
                        </Link>
                      )
                    })}
                  </nav>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      <div className="flex min-h-screen w-full lg:h-full">
        <aside className={cn('hidden shrink-0 border-r border-[var(--border)] bg-[var(--sidebar)] transition-all duration-200 lg:flex lg:h-full lg:flex-col', sidebarWidth)}>
          <div className="flex h-16 items-center gap-3 border-b border-[var(--border)] px-4">
            <div className="flex h-9 w-9 items-center justify-center text-[var(--accent)]">
              <Logo size={32} className="text-[var(--accent)]" />
            </div>
            {!collapsed && (
              <div>
                <h1 className="text-sm font-semibold tracking-[-0.03em]">Xray Pilot</h1>
                <p className="mt-0.5 text-[11px] uppercase tracking-[0.14em] text-faint">Control Panel</p>
              </div>
            )}
          </div>

          <div className="flex-1 overflow-auto px-3 py-4">
            {navGroups.map((group) => (
              <div key={group.title} className="mb-6">
                {!collapsed && (
                  <div className="mb-2 px-2 text-[11px] font-medium uppercase tracking-[0.14em] text-faint">
                    {group.title}
                  </div>
                )}
                <nav className="space-y-1">
                  {group.items.map((item) => {
                    const active = location.pathname.startsWith(item.path)
                    return (
                      <Link
                        key={item.path}
                        to={item.path}
                        className={cn(
                          'group flex items-center rounded-md px-3 py-2.5 text-sm transition-all',
                          collapsed ? 'justify-center' : 'gap-3',
                          active
                            ? 'bg-[var(--accent-soft)] text-[var(--accent)]'
                            : 'text-soft hover:bg-[var(--sidebar-hover)] hover:text-[var(--text)]'
                        )}
                      >
                        <item.icon className={cn('h-4 w-4 transition-transform group-hover:scale-105', active && 'scale-105')} />
                        {!collapsed && <span className="flex-1 font-medium">{item.label}</span>}
                        {!collapsed && <ChevronRight className={cn('h-4 w-4 transition', active ? 'opacity-100' : 'opacity-0 group-hover:opacity-40')} />}
                      </Link>
                    )
                  })}
                </nav>
              </div>
            ))}
          </div>

          <div className="border-t border-[var(--border)] p-3">
            {!collapsed && (
              <div className="px-1 pb-2 text-[11px] uppercase tracking-[0.12em] text-faint">
                © 2026 Xray Pilot. {APP_VERSION}
              </div>
            )}
            <button
              onClick={() => setCollapsed((s) => !s)}
              className={cn(
                'flex w-full items-center rounded-md px-3 py-2 text-sm text-soft transition hover:bg-[var(--sidebar-hover)] hover:text-[var(--text)]',
                collapsed ? 'justify-center' : 'gap-2'
              )}
            >
              {collapsed ? <ChevronRight className="h-4 w-4" /> : <ChevronLeft className="h-4 w-4" />}
              {!collapsed && '收起侧栏'}
            </button>
          </div>
        </aside>

        <main className="min-w-0 flex-1 lg:h-full">
          <div className="flex min-h-screen flex-col lg:h-full lg:min-h-0">
            <header className="hidden h-16 items-center justify-between border-b border-[var(--border)] bg-[color:var(--panel-strong)]/92 px-6 backdrop-blur-xl lg:flex">
              <div className="flex flex-col justify-center leading-tight">
                <div className="text-base font-semibold tracking-[-0.03em]">{currentView}</div>
                <div className="mt-0.5 text-xs text-faint">{currentSubTitle}</div>
              </div>
              <div className="flex h-full items-center gap-2.5">
                <label className="relative hidden lg:block">
                  <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-faint" />
                  <input
                    placeholder="搜索节点、用户..."
                    className="h-9 w-64 rounded-md border border-[var(--border)] bg-[var(--panel)] pl-10 pr-14 text-sm text-[var(--text)] placeholder:text-faint focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
                  />
                  <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-[10px] uppercase tracking-[0.12em] text-faint">
                    ⌘K
                  </span>
                </label>
                <a
                  href="https://github.com/imrui/xray-pilot"
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex h-9 items-center gap-1.5 rounded-md border border-[var(--border)] bg-[var(--panel)] px-2.5 text-xs text-soft transition hover:text-[var(--text)]"
                  title="GitHub 仓库"
                >
                  <GitHubMark className="h-4 w-4" />
                  GitHub
                </a>
                {/* <button className="rounded-md border border-[var(--border)] bg-[var(--panel)] p-2 text-soft transition hover:text-[var(--text)]">
                  <RefreshCw className="h-4 w-4" />
                </button> */}
                <button
                  onClick={toggleTheme}
                  className="inline-flex h-9 w-9 items-center justify-center rounded-md border border-[var(--border)] bg-[var(--panel)] text-soft transition hover:text-[var(--text)]"
                  title="切换主题"
                >
                  {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
                </button>
                {/* <button className="relative rounded-md border border-[var(--border)] bg-[var(--panel)] p-2 text-soft transition hover:text-[var(--text)]">
                  <Bell className="h-4 w-4" />
                  <span className="absolute right-1.5 top-1.5 h-2 w-2 rounded-full bg-[var(--accent)]" />
                </button> */}

                <DropdownMenu.Root>
                  <DropdownMenu.Trigger asChild>
                    <button className="flex h-9 w-9 items-center justify-center rounded-full bg-[var(--accent-soft)] text-[var(--accent)] ring-1 ring-[var(--border)]">
                      <UserRound className="h-5 w-5" />
                    </button>
                  </DropdownMenu.Trigger>
                  <DropdownMenu.Portal>
                    <DropdownMenu.Content
                      align="end"
                      sideOffset={8}
                      className="z-50 min-w-[160px] rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-1.5 shadow-[var(--shadow-card)]"
                    >
                      <DropdownMenu.Item className="cursor-default rounded-md px-2.5 py-2 text-sm text-soft outline-none">
                        个人中心
                      </DropdownMenu.Item>
                      <DropdownMenu.Separator className="my-1 h-px bg-[var(--border)]" />
                      <DropdownMenu.Item
                        onSelect={handleLogout}
                        className="flex cursor-pointer select-none items-center gap-2 rounded-md px-2.5 py-2 text-sm text-[var(--danger)] outline-none transition hover:bg-[var(--danger-soft)]"
                      >
                        <LogOut className="h-4 w-4" />
                        退出登录
                      </DropdownMenu.Item>
                    </DropdownMenu.Content>
                  </DropdownMenu.Portal>
                </DropdownMenu.Root>
              </div>
            </header>

            <div className="flex-1 px-4 py-4 md:px-6 md:py-5 lg:min-h-0 lg:overflow-y-auto">
              <div className="min-h-full">
                {syncSummary && <SyncReminderBanner summary={syncSummary} />}
                <Outlet />
              </div>
            </div>
          </div>
        </main>
      </div>
    </div>
  )
}
