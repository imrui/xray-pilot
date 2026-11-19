import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { systemApi } from '@/lib/api'
import { Btn, Field, Switch } from '@/components/ui/Form'
import { Badge } from '@/components/ui/Badge'
import { pushToast } from '@/lib/notify'
import { SecretField, Section } from './shared'
import { useSettingsForm } from './context'
import AlertSettings from './AlertSettings'

// 订阅与通知：订阅链接前缀 / 备注模板 + 飞书机器人 + 节点告警
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
      <Section title="订阅配置" description="通过反向代理部署时填写公网地址，留空时使用请求的 Host。">
        <Field
          label="订阅链接前缀"
          value={form['subscription.base_url'] ?? ''}
          onChange={f('subscription.base_url')}
          placeholder="https://your-domain.com"
        />
        <div>
          <Field
            label="节点备注格式"
            value={form['subscription.remark_format'] ?? ''}
            onChange={f('subscription.remark_format')}
            placeholder="{node_name} ({username}) [{protocol} - {transport}]"
            aria-describedby="subscription-remark-tokens"
          />
          <p id="subscription-remark-tokens" className="mt-2 text-sm text-soft">可用占位符 <code className="break-words">{'{node_name} {username} {protocol} {transport} {region}'}</code></p>
        </div>
      </Section>

      <Section title="内网中转订阅" description="向有效用户展示个人内网中转订阅地址，并随飞书订阅消息发送。" actions={<label className="inline-flex items-center gap-2 text-sm">启用内网中转订阅<Switch checked={form['subscription.relay_enabled'] === 'true'} onChange={value => set('subscription.relay_enabled', String(value))} /></label>}>
        <Field label="内网中转订阅地址前缀" value={form['subscription.relay_prefix'] ?? ''} onChange={f('subscription.relay_prefix')} placeholder="http://192.168.0.2:2096/sub/" />
      </Section>
      <Section
        title="飞书机器人"
        description="关闭后保留机器人凭据，订阅链接、订阅页和二维码仍可使用。"
        actions={
          <div className="flex flex-wrap items-center gap-3">
            <Btn variant="secondary" loading={testFeishu.isPending || feishuStatusLoading} onClick={() => testFeishu.mutate()}>
              检查已保存配置
            </Btn>
            <label className="inline-flex cursor-pointer items-center gap-2 text-sm font-medium text-soft">
              启用飞书
              <Switch checked={feishuEnabled} onChange={(next) => set('feishu.enabled', next ? 'true' : 'false')} />
            </label>
          </div>
        }
      >
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
              <Field label="飞书回调基址（公网 HTTPS 地址）" value={form['feishu.base_url'] ?? ''} onChange={f('feishu.base_url')} placeholder="https://your-domain.com" />
              <Field label="机器人名称" value={form['feishu.bot_name'] ?? ''} onChange={f('feishu.bot_name')} placeholder="xray-pilot" />
            </div>
            <div className="border-t border-[var(--border)] pt-4">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-semibold">已保存配置</span>
                {feishuStatus ? (
                  <Badge
                    label={!feishuStatus.enabled ? '未启用' : feishuStatus.configured ? '已就绪' : '待补全'}
                    variant={!feishuStatus.enabled ? 'gray' : feishuStatus.configured ? 'green' : 'yellow'}
                  />
                ) : <span className="text-sm text-soft">{feishuStatusLoading ? '读取中…' : '读取失败'}</span>}
              </div>
              <div className="mt-3 space-y-2 text-sm text-soft">
                {feishuStatus && <p className="break-all">Webhook 地址：{feishuStatus.webhook_url || '填写并保存回调基址后生成 /api/feishu/events 地址'}</p>}
                {feishuStatus?.enabled && !feishuStatus.configured && (
                  <p>缺少配置项：{(feishuStatus.missing_keys ?? []).join('、') || '请补全基础配置'}</p>
                )}
              </div>
            </div>
          </div>
        )}
      </Section>

      <AlertSettings />
    </>
  )
}
