import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface ThemeState {
  theme: 'light' | 'dark'
  toggleTheme: () => void
  setTheme: (t: 'light' | 'dark') => void
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set) => ({
      theme: 'light',
      toggleTheme: () => set((state) => {
        const nextTheme = state.theme === 'light' ? 'dark' : 'light'
        if (nextTheme === 'dark') {
          document.documentElement.classList.add('dark')
        } else {
          document.documentElement.classList.remove('dark')
        }
        return { theme: nextTheme }
      }),
      setTheme: (t) => set(() => {
        if (t === 'dark') {
          document.documentElement.classList.add('dark')
        } else {
          document.documentElement.classList.remove('dark')
        }
        return { theme: t }
      })
    }),
    { name: 'theme-storage' }
  )
)

export function initTheme() {
  const store = localStorage.getItem('theme-storage')
  if (store && JSON.parse(store)?.state?.theme === 'dark') {
    document.documentElement.classList.add('dark')
  } else if (!store && window.matchMedia('(prefers-color-scheme: dark)').matches) {
    document.documentElement.classList.add('dark')
  }
}
