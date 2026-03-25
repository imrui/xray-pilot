import { Link, useLocation, Outlet } from 'react-router-dom'
import { cn } from '@/lib/utils'
import { Users, Layers, Server, ScrollText, Settings, LogOut, Shield } from 'lucide-react'
import { useAuthStore } from '@/store/auth'
import { useNavigate } from 'react-router-dom'

const navItems = [
  { path: '/users',    label: '用户管理', icon: Users },
  { path: '/groups',   label: '分组管理', icon: Layers },
  { path: '/nodes',    label: '节点管理', icon: Server },
  { path: '/profiles', label: '协议配置', icon: Shield },
  { path: '/logs',     label: '操作日志', icon: ScrollText },
  { path: '/settings', label: '系统设置', icon: Settings },
]

export default function Layout() {
  const location = useLocation()
  const navigate = useNavigate()
  const logout = useAuthStore((s) => s.logout)

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  return (
    <div className="flex h-screen bg-slate-50">
      {/* 左侧导航栏 */}
      <aside className="w-56 bg-slate-950 text-slate-300 flex flex-col shrink-0">
        {/* Logo */}
        <div className="h-16 flex items-center px-5 border-b border-slate-800">
          <div className="flex items-center gap-2.5">
            <div className="w-7 h-7 rounded-lg bg-indigo-600 flex items-center justify-center">
              <Shield className="w-4 h-4 text-white" />
            </div>
            <div>
              <p className="text-sm font-semibold text-white leading-tight">xray-pilot</p>
              <p className="text-xs text-slate-500 leading-tight">节点管理控制台</p>
            </div>
          </div>
        </div>

        {/* 导航项 */}
        <nav className="flex-1 px-2 py-3 space-y-0.5">
          {navItems.map((item) => {
            const Icon = item.icon
            const active = location.pathname.startsWith(item.path)
            return (
              <Link
                key={item.path}
                to={item.path}
                className={cn(
                  'flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-colors',
                  active
                    ? 'bg-indigo-600 text-white'
                    : 'text-slate-400 hover:bg-slate-800 hover:text-slate-200'
                )}
              >
                <Icon className="w-4 h-4 shrink-0" />
                {item.label}
              </Link>
            )
          })}
        </nav>

        {/* 退出 */}
        <div className="px-2 py-3 border-t border-slate-800">
          <button
            onClick={handleLogout}
            className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium text-slate-400 hover:bg-slate-800 hover:text-slate-200 w-full transition-colors"
          >
            <LogOut className="w-4 h-4 shrink-0" />
            退出登录
          </button>
        </div>
      </aside>

      {/* 主内容区 */}
      <main className="flex-1 overflow-auto">
        <Outlet />
      </main>
    </div>
  )
}
