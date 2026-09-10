import { create } from 'zustand'

export type AdminRole = 'super_admin' | 'admin'

interface AuthState {
  token: string | null
  username: string | null
  role: AdminRole | null
  /** 登录成功后写入 token 与当前管理员身份 */
  setSession: (token: string, username: string, role: AdminRole) => void
  logout: () => void
}

const KEY_TOKEN = 'token'
const KEY_USERNAME = 'admin_username'
const KEY_ROLE = 'admin_role'

function readRole(): AdminRole | null {
  const raw = localStorage.getItem(KEY_ROLE)
  return raw === 'super_admin' || raw === 'admin' ? raw : null
}

export const useAuthStore = create<AuthState>((set) => ({
  token: localStorage.getItem(KEY_TOKEN),
  username: localStorage.getItem(KEY_USERNAME),
  role: readRole(),
  setSession: (token, username, role) => {
    localStorage.setItem(KEY_TOKEN, token)
    localStorage.setItem(KEY_USERNAME, username)
    localStorage.setItem(KEY_ROLE, role)
    set({ token, username, role })
  },
  logout: () => {
    localStorage.removeItem(KEY_TOKEN)
    localStorage.removeItem(KEY_USERNAME)
    localStorage.removeItem(KEY_ROLE)
    set({ token: null, username: null, role: null })
  },
}))
