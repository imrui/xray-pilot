import { useAuthStore } from '@/store/auth'
import { AdminManager } from '@/components/AdminManager'
import { Section } from './shared'

// 管理员：仅超级管理员可操作；普通管理员直接访问 URL 时给出提示（后端同样 403）
export default function AdminsSection() {
  const role = useAuthStore((s) => s.role)
  if (role !== 'super_admin') {
    return (
      <Section title="管理员账号" description="仅超级管理员可以管理登录账号。">
        <p className="text-sm text-soft">当前账号是普通管理员。如需修改自己的密码，请使用右上角头像菜单中的「修改密码」。</p>
      </Section>
    )
  }
  return <AdminManager />
}
