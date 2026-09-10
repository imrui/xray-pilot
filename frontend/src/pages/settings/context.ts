import { useOutletContext } from 'react-router-dom'

export type SettingsMap = Record<string, string>

// 设置页各表单分组共享的表单上下文：由 SettingsLayout 持有，子路由通过 useSettingsForm() 读写
export interface SettingsFormContext {
  form: SettingsMap
  setForm: React.Dispatch<React.SetStateAction<SettingsMap>>
  /** 便捷绑定：f('key') 返回 input onChange */
  f: (key: string) => (e: React.ChangeEvent<HTMLInputElement>) => void
  /** 便捷设值：set('key', value) */
  set: (key: string, value: string) => void
}

export function useSettingsForm() {
  return useOutletContext<SettingsFormContext>()
}

// 设置页分组路由定义（单一来源：侧栏导航与 App 路由都从这里取）
export const SETTINGS_SECTIONS = [
  { key: 'status', label: '运行状态', hint: '系统信息与部署诊断', form: false },
  { key: 'sync', label: '节点与同步', hint: 'SSH、Xray 日志、调度周期', form: true },
  { key: 'subscription', label: '订阅与通知', hint: '订阅链接、飞书机器人', form: true },
  { key: 'backup', label: '数据备份', hint: '备份周期与快照文件', form: true },
  { key: 'admins', label: '管理员', hint: '面板登录账号', form: false, superAdminOnly: true },
] as const

export type SettingsSectionKey = (typeof SETTINGS_SECTIONS)[number]['key']
