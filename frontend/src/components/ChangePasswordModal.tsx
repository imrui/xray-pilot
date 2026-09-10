import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { authApi } from '@/lib/api'
import { Modal } from '@/components/ui/Modal'
import { Btn, Field } from '@/components/ui/Form'
import { pushToast } from '@/lib/notify'

// 修改当前登录管理员自己的密码（需旧密码）。所有角色可用。
export function ChangePasswordModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')

  const reset = () => {
    setOldPassword('')
    setNewPassword('')
    setConfirmPassword('')
  }

  const change = useMutation({
    mutationFn: () => authApi.changePassword(oldPassword, newPassword),
    onSuccess: () => {
      pushToast({ title: '密码已更新', description: '下次登录请使用新密码。', variant: 'success' })
      reset()
      onClose()
    },
  })

  const mismatch = confirmPassword !== '' && newPassword !== confirmPassword
  const canSubmit = oldPassword !== '' && newPassword.length >= 6 && newPassword === confirmPassword

  return (
    <Modal
      open={open}
      onClose={() => {
        reset()
        onClose()
      }}
      title="修改密码"
      size="sm"
      footer={
        <>
          <Btn variant="secondary" onClick={() => { reset(); onClose() }}>取消</Btn>
          <Btn loading={change.isPending} disabled={!canSubmit} onClick={() => change.mutate()}>保存</Btn>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="当前密码" type="password" value={oldPassword} onChange={(e) => setOldPassword(e.target.value)} autoComplete="current-password" />
        <Field label="新密码（至少 6 位）" type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} autoComplete="new-password" />
        <Field
          label="确认新密码"
          type="password"
          value={confirmPassword}
          onChange={(e) => setConfirmPassword(e.target.value)}
          autoComplete="new-password"
          error={mismatch ? '两次输入的新密码不一致' : undefined}
        />
      </div>
    </Modal>
  )
}
