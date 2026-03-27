import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type ThemeMode = 'light' | 'dark'

interface ThemeState {
  theme: ThemeMode
  toggleTheme: () => void
  setTheme: (t: ThemeMode) => void
}

function applyTheme(mode: ThemeMode) {
  if (mode === 'dark') {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set) => ({
      theme: 'dark',
      toggleTheme: () =>
        set((state) => {
          const next: ThemeMode = state.theme === 'dark' ? 'light' : 'dark'
          applyTheme(next)
          return { theme: next }
        }),
      setTheme: (t) =>
        set(() => {
          applyTheme(t)
          return { theme: t }
        }),
    }),
    { name: 'theme-storage' }
  )
)

export function initTheme() {
  const raw = localStorage.getItem('theme-storage')
  let mode: ThemeMode = 'dark'
  if (raw) {
    try {
      const parsed = JSON.parse(raw)
      const saved = parsed?.state?.theme as ThemeMode | undefined
      if (saved === 'light' || saved === 'dark') {
        mode = saved
      }
    } catch {
      mode = 'dark'
    }
  }
  applyTheme(mode)
}
