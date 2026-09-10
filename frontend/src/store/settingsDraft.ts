import { create } from 'zustand'

// 设置页未保存改动的跨组件信号：SettingsLayout 写入，侧栏二级菜单在切换分组前读取并拦截
interface SettingsDraftState {
  dirty: boolean
  setDirty: (dirty: boolean) => void
  /** 由 SettingsLayout 注册：丢弃当前草稿 */
  discard: () => void
  setDiscard: (fn: () => void) => void
}

export const useSettingsDraft = create<SettingsDraftState>((set) => ({
  dirty: false,
  setDirty: (dirty) => set({ dirty }),
  discard: () => {},
  setDiscard: (fn) => set({ discard: fn }),
}))
