import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { systemApi } from '@/lib/api'
import { Btn, Field, Switch } from '@/components/ui/Form'
import { Badge } from '@/components/ui/Badge'
import { pushToast } from '@/lib/notify'
import { SecretField, Section } from './shared'
import { useSettingsForm } from './context'

// 订阅与通知：订阅链接前缀 / 备注模板 + 飞书机器人
export default function SubscriptionSection() {
  const { form, f, set } = useSettingsForm()
  const [secretVisibility, setSecretVisibility] = useState<Record<string, boolean>>({})
  const toggleSecretVisibility = (key: string) => setSecretVisibility((prev) => ({ ...prev, [key]: !prev[key] }))

  const { data: feishuStatus, refetch: refetchFeishuStatus, isFetching: feishuStatusLoading } = useQuery({
    queryKey: ['system-feishu-status'],
    queryFn: () => systemApi.getFeishuStatus().then((r) => r.data.data!),
  })

  const testFeishu = useMutation({
    mutationFn: () => systemApi.testFeishuConfig(),
    onSuccess: (res) => {
      const status = res.data.data
      pushToast({
        title: '飞书配置检查通过',
        description: status?.webhook_url ? `Webhook 地址：${status.webhook_url}` : '飞书基础配置已就绪。',
        variant: 'success',
      })
      void refetchFeishuStatus()
    },
    onError: (e: Error) => {
      pushToast({ title: '飞书配置检查失败', description: e.message, variant: 'warning' })
      void refetchFeishuStatus()
    },
  })

  const feishuEnabled = (form['feishu.enabled'] ?? 'false') === 'true'

  return (
    <>
      <Section title="订阅配置" description="显式配置公网访问地址与备注模板，避免订阅链接依赖反代环境推断。">
        <Field
          label="订阅链接前缀（留空则自动从请求 Host 获取）"
          value={form['subscription.base_url'] ?? ''}
          onChange={f('subscription.base_url')}
          placeholder="https://your-domain.com"
        />
        <p className="text-xs text-soft">服务部署在 Nginx、CDN 或 HTTPS 反向代理后时，建议显式填写公网访问地址。</p>
        <div>
          <Field
            label="节点备注格式"
            value={form['subscription.remark_format'] ?? ''}
            onChange={f('subscription.remark_format')}
            placeholder="{node_name} ({username}) [{protocol} - {transport}]"
          />
          <p className="mt-2 text-xs text-soft">可用占位符：{'{node_name}'} {'{username}'} {'{protocol}'} {'{transport}'} {'{region}'}</p>
        </div>
      </Section>

      <Section
        title="飞书机器人"
        description="可选能力。未配置时不影响订阅链接、订阅页和二维码的正常使用。"
        actions={
          <div className="flex items-center gap-3 rounded-full border border-[var(--border)] bg-[var(--panel-strong)] px-3 py-2">
            <span className="text-sm font-medium text-soft">启用飞书</span>
            <Switch checked={feishuEnabled} onChange={(next) => set('feishu.enabled', next ? 'true' : 'false')} />
          </div>
        }
      >
        <div className="border-t border-[var(--border)]">
          <div className="flex flex-col gap-3 py-4 md:flex-row md:items-center md:justify-between">
            <div className="space-y-1">
              <div className="text-sm font-semibold">飞书集成</div>
              <p className="text-xs text-soft">关闭时折叠配置区域且不启用飞书消息能力；已保存的飞书信息仍会保留。</p>
            </div>
            <div className="flex items-center gap-3">
              <Btn variant="secondary" loading={testFeishu.isPending || feishuStatusLoading} onClick={() => testFeishu.mutate()}>
                检查配置
              </Btn>
              {feishuEnabled ? <ChevronDown className="h-4 w-4 text-faint" /> : <ChevronRight className="h-4 w-4 text-faint" />}
            </div>
          </div>

          {feishuEnabled && (
            <div className="space-y-4 border-t border-[var(--border)] pt-4">
              <div className="grid gap-4 md:grid-cols-2">
                <SecretField
                  label="App ID"
                  value={form['feishu.app_id'] ?? ''}
                  onChange={f('feishu.app_id')}
                  placeholder="cli_xxx"
                  revealed={Boolean(secretVisibility['feishu.app_id'])}
                  onToggleReveal={() => toggleSecretVisibility('feishu.app_id')}
                />
                <SecretField
                  label="App Secret"
                  value={form['feishu.app_secret'] ?? ''}
                  onChange={f('feishu.app_secret')}
                  placeholder="填写飞书应用密钥"
                  revealed={Boolean(secretVisibility['feishu.app_secret'])}
                  onToggleReveal={() => toggleSecretVisibility('feishu.app_secret')}
                />
              </div>
              <div className="grid gap-4 md:grid-cols-2">
                <SecretField
                  label="Verification Token"
                  value={form['feishu.verification_token'] ?? ''}
                  onChange={f('feishu.verification_token')}
                  placeholder="事件订阅校验 Token"
                  revealed={Boolean(secretVisibility['feishu.verification_token'])}
                  onToggleReveal={() => toggleSecretVisibility('feishu.verification_token')}
                />
                <SecretField
                  label="Encrypt Key"
                  value={form['feishu.encrypt_key'] ?? ''}
                  onChange={f('feishu.encrypt_key')}
                  placeholder="消息加密 Key（可选）"
                  revealed={Boolean(secretVisibility['feishu.encrypt_key'])}
                  onToggleReveal={() => toggleSecretVisibility('feishu.encrypt_key')}
                />
              </div>
              <div className="grid gap-4 md:grid-cols-2">
                <Field label="飞书回调基址" value={form['feishu.base_url'] ?? ''} onChange={f('feishu.base_url')} placeholder="https://your-domain.com" />
                <Field label="机器人名称" value={form['feishu.bot_name'] ?? ''} onChange={f('feishu.bot_name')} placeholder="xray-pilot" />
              </div>
              <div className="border-t border-[var(--border)] pt-4">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-semibold">当前状态</span>
                  <Badge
                    label={!feishuStatus?.enabled ? '未启用' : feishuStatus.configured ? '已就绪' : '待补全'}
                    variant={!feishuStatus?.enabled ? 'gray' : feishuStatus.configured ? 'green' : 'yellow'}
                  />
                </div>
                <div className="mt-3 space-y-2 text-sm text-soft">
                  <p>机器人名称：{feishuStatus?.bot_name || form['feishu.bot_name'] || 'xray-pilot'}</p>
                  <p>Webhook 地址：{feishuStatus?.webhook_url || '请先填写飞书回调基址'}</p>
                  {feishuStatus?.enabled && !feishuStatus.configured && (
                    <p>缺少配置项：{(feishuStatus.missing_keys ?? []).join('、') || '请补全基础配置'}</p>
                  )}
                </div>
              </div>
              <p className="text-xs text-soft">建议对外提供固定 HTTPS 地址，webhook 路径为 <code>/api/feishu/events</code>。</p>
            </div>
          )}
        </div>
      </Section>
    </>
  )
}
