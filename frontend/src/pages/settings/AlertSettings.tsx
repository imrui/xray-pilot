import { useMutation, useQuery } from '@tanstack/react-query'
import { RefreshCw, Send } from 'lucide-react'
import { systemApi } from '@/lib/api'
import { Btn, Field, SelectField, Switch } from '@/components/ui/Form'
import { pushToast } from '@/lib/notify'
import { useSettingsDraft } from '@/store/settingsDraft'
import { Section } from './shared'
import { useSettingsForm } from './context'

// 告警类型（与后端 entity.AlertKind* 一致）
const ALERT_KINDS = [
  { key: 'node_unreachable', label: '服务器不可达', hint: 'SSH 端口连接失败' },
  { key: 'xray_down', label: 'Xray 端口不通', hint: 'SSH 可达但代理端口（TCP）不通' },
  { key: 'sync_failed', label: '配置同步失败', hint: '失败 1 次即告警，下次同步成功恢复' },
  { key: 'panel_unreachable', label: '面板到全部节点不可达', hint: '疑似面板出网或跨境链路故障' },
] as const

// 节点异常告警：复用飞书应用机器人发群卡片（v0.5.1）
export default function AlertSettings() {
  const { form, f, set } = useSettingsForm()
  const dirty = useSettingsDraft((s) => s.dirty)

  const feishuEnabled = (form['feishu.enabled'] ?? 'false') === 'true'
  const alertEnabled = (form['alert.enabled'] ?? 'false') === 'true'
  const chatID = form['alert.feishu_chat_id'] ?? ''
  const kinds = new Set((form['alert.kinds'] ?? '').split(',').map((k) => k.trim()).filter(Boolean))

  // 手动拉取：未开通 im:chat:readonly 时接口会报错，不在进页面时自动请求
  const chats = useQuery({
    queryKey: ['system-feishu-chats'],
    queryFn: () => systemApi.listFeishuChats().then((r) => r.data.data ?? []),
    enabled: false,
    retry: false,
  })

  const testAlert = useMutation({
    mutationFn: () => systemApi.testAlert(),
    onSuccess: () => pushToast({ title: '测试消息已发送', description: '请到飞书群中查看。', variant: 'success' }),
  })

  const toggleKind = (key: string, on: boolean) => {
    const next = new Set(kinds)
    if (on) next.add(key)
    else next.delete(key)
    set('alert.kinds', ALERT_KINDS.map((k) => k.key).filter((k) => next.has(k)).join(','))
  }

  const healthInterval = Number(form['scheduler.health_check_interval'] ?? '120')
  const failThreshold = Number(form['alert.fail_threshold'] ?? '3')
  const delayMinutes = Math.ceil((healthInterval * failThreshold) / 60)

  const chatOptions = (chats.data ?? []).map((c) => ({ value: c.chat_id, label: c.name || c.chat_id }))
  if (chatID && !chatOptions.some((o) => o.value === chatID)) {
    chatOptions.unshift({ value: chatID, label: `${chatID}（不在机器人群列表中）` })
  }

  return (
    <Section
      title="节点告警"
      description="节点异常与恢复时，由飞书机器人推送卡片消息到指定群。同一轮检测的多条变化合并为一条消息。"
      actions={
        <div className="flex items-center gap-3 rounded-full border border-[var(--border)] bg-[var(--panel-strong)] px-3 py-2">
          <span className="text-sm font-medium text-soft">启用告警</span>
          <Switch checked={alertEnabled} onChange={(next) => set('alert.enabled', next ? 'true' : 'false')} />
        </div>
      }
    >
      {!feishuEnabled && (
        <div className="border-l-2 border-[var(--warning)] pl-3 text-xs text-soft">
          告警复用上方的飞书应用机器人，需先启用飞书集成并填写 App ID / App Secret。
        </div>
      )}

      {alertEnabled && (
        <div className="space-y-4 border-t border-[var(--border)] pt-4">
          <div className="flex flex-col gap-3 md:flex-row md:items-end">
            <div className="flex-1">
              {chats.isSuccess ? (
                <SelectField
                  label="告警群"
                  value={chatID}
                  onChange={(v) => set('alert.feishu_chat_id', v)}
                  options={chatOptions}
                  placeholder="选择机器人所在的群"
                />
              ) : (
                <Field label="告警群 chat_id" value={chatID} onChange={f('alert.feishu_chat_id')} placeholder="oc_xxx，或点击右侧获取群列表" />
              )}
            </div>
            <Btn variant="secondary" loading={chats.isFetching} disabled={!feishuEnabled} onClick={() => void chats.refetch()}>
              <RefreshCw className="h-4 w-4" />
              获取群列表
            </Btn>
          </div>
          <p className="text-xs text-soft">
            需先把机器人拉进目标群；获取群列表需为飞书应用开通 <code>im:chat:readonly</code> 权限，未开通时可手动填写 chat_id。
          </p>

          <div className="grid gap-4 md:grid-cols-3">
            <Field label="连续失败几次触发" type="number" min={1} value={form['alert.fail_threshold'] ?? '3'} onChange={f('alert.fail_threshold')} />
            <Field label="连续成功几次恢复" type="number" min={1} value={form['alert.recover_threshold'] ?? '2'} onChange={f('alert.recover_threshold')} />
            <Field label="未恢复重复提醒（小时，0 不重复）" type="number" min={0} value={form['alert.repeat_hours'] ?? '0'} onChange={f('alert.repeat_hours')} />
          </div>
          {healthInterval > 0 ? (
            <p className="text-xs text-soft">
              按当前健康检测间隔 {healthInterval} 秒，异常约 {delayMinutes} 分钟后告警。检测间隔在「节点与同步」中调整。
            </p>
          ) : (
            <div className="border-l-2 border-[var(--warning)] pl-3 text-xs text-soft">
              健康检测已禁用（间隔为 0），除「配置同步失败」外的告警都不会触发。
            </div>
          )}

          <div className="border-t border-[var(--border)] pt-4">
            <div className="mb-3 text-sm font-semibold">告警类型</div>
            <div className="grid gap-3 md:grid-cols-2">
              {ALERT_KINDS.map((k) => (
                <label key={k.key} className="flex cursor-pointer items-start gap-3">
                  <input
                    type="checkbox"
                    checked={kinds.has(k.key)}
                    onChange={(e) => toggleKind(k.key, e.target.checked)}
                    className="mt-0.5 h-4 w-4 rounded border-[var(--border-strong)]"
                  />
                  <span>
                    <span className="block text-sm font-medium">{k.label}</span>
                    <span className="block text-xs text-soft">{k.hint}</span>
                  </span>
                </label>
              ))}
            </div>
          </div>

          <div className="flex flex-col gap-3 border-t border-[var(--border)] pt-4 md:flex-row md:items-center md:justify-between">
            <p className="text-xs text-soft">
              {dirty ? '有未保存的改动，测试消息使用已保存的配置，请先保存。' : '向已保存的告警群发送一条测试消息。'}
            </p>
            <Btn variant="secondary" loading={testAlert.isPending} disabled={dirty || !feishuEnabled} onClick={() => testAlert.mutate()}>
              <Send className="h-4 w-4" />
              发送测试消息
            </Btn>
          </div>
        </div>
      )}
    </Section>
  )
}
