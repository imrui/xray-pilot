import { useMutation, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { Btn, FieldGroup } from '@/components/ui/Form'
import { useConfirm } from '@/lib/confirm-context'
import { dnsApi, dnsStatus, type DNSBinding } from '@/lib/dns'
import { useAuthStore } from '@/store/auth'

interface NodeDNSProps {
  id: number
  binding?: DNSBinding
  changed: boolean
  refreshing: boolean
  onRefresh: () => void
}

export function NodeDNS({ id, binding, changed, refreshing, onRefresh }: NodeDNSProps) {
  const admin = useAuthStore(s => s.role === 'super_admin')
  const qc = useQueryClient()
  const confirm = useConfirm()
  const action = useMutation({
    mutationFn: async (kind: 'bind' | 'unbind' | 'retry') => {
      if (kind === 'retry') return dnsApi.retry(id)
      if (kind === 'unbind') {
        if (await confirm({ title: '解除 DNS 绑定？', description: 'Cloudflare 将保留这条记录。' })) await dnsApi.unbind(id)
        return
      }
      const p = await dnsApi.preview(id)
      const difference = p.remote_ip !== p.node_ip ? '绑定后可手动同步到节点 IP。' : ''
      if (await confirm({ title: '确认绑定 DNS 记录', description: `${p.domain}（${p.type}），远端 ${p.remote_ip}，节点 ${p.node_ip}。${difference}`, confirmText: '绑定记录' })) await dnsApi.bind(id, p)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['dns-bindings'] }),
  })
  return <FieldGroup title="节点 DNS 同步">
    {binding ? <p className="break-all text-sm">{binding.domain}（{binding.type}） · {dnsStatus[binding.status]}<br />最近确认 {binding.confirmed_ip}，期望 {binding.desired_ip}<br />{binding.message}</p> : <p className="text-sm text-soft">未绑定 DNS 记录</p>}
    <div className="flex flex-wrap gap-2">
      <Btn variant="secondary" disabled={refreshing} onClick={onRefresh}>
        <RefreshCw aria-hidden="true" className={`h-4 w-4${refreshing ? ' animate-spin' : ''}`} />刷新
      </Btn>
      {admin && <>
        <Btn variant="secondary" disabled={changed || action.isPending} onClick={() => action.mutate('bind')}>{binding ? '重新绑定' : '预览并绑定'}</Btn>
        {binding && ['review', 'failed', 'synced'].includes(binding.status) &&
          <Btn variant="secondary" disabled={changed || action.isPending} onClick={() => action.mutate('retry')}>手动同步</Btn>}
        {binding && <Btn variant="ghost" disabled={action.isPending} onClick={() => action.mutate('unbind')}>解除绑定</Btn>}
        {changed && <span className="text-sm text-soft">请先保存连接地址</span>}
      </>}
    </div>
  </FieldGroup>
}
