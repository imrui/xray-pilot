import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Copy } from 'lucide-react'
import { nodeApi, profileApi } from '@/lib/api'
import { fetchAllPages } from '@/lib/pagination'
import { copyText } from '@/lib/clipboard'
import { pushToast } from '@/lib/notify'
import { routingGet, useRoutingOverview } from '@/lib/routing'
import { basicRoutingDescription, routingLabel, configConfirmed } from '@/lib/routing-model'
import type { EffectiveRouting } from '@/lib/routing-model'
import type { Node, NodeKey, InboundProfile } from '@/types'
import { Modal } from '@/components/ui/Modal'
import { Badge } from '@/components/ui/Badge'
import { Btn } from '@/components/ui/Form'

export function PreviewConfigModal({
  node,
  activeProfiles,
  onClose,
}: {
  node: Pick<Node, 'id' | 'name'>
  activeProfiles?: InboundProfile[]
  onClose: () => void
}) {
  const profiles = useQuery({
    queryKey: ['profiles', 'preview'],
    queryFn: async () => {
      const all = await fetchAllPages((page, pageSize) =>
        profileApi.list({ page, page_size: pageSize }).then((r) => r.data.data!),
      )
      return all.filter((profile) => profile.active)
    },
    enabled: activeProfiles === undefined,
  })
  const availableProfiles = activeProfiles ?? profiles.data
  const [copied, setCopied] = useState(false)
  const [tab, setTab] = useState<'config' | 'source' | 'comparison'>('config')
  const source = useQuery({
    queryKey: ['routing-effective', node.id],
    queryFn: () =>
      routingGet<{ routing: EffectiveRouting; desired_hash: string }>(
        `/nodes/${node.id}/effective-config`,
      ),
  })
  const state = useQuery({
    queryKey: ['config-state', node.id],
    queryFn: () =>
      routingGet<{
        runtime_hash: string
        disk_hash: string
        verified_at?: string
        verification: string
        last_error: string
      }>(`/nodes/${node.id}/config-state`),
    enabled: tab === 'comparison',
  })
  const overview = useRoutingOverview()
  const { data, isLoading, error } = useQuery({
    queryKey: ['previewConfig', node.id],
    queryFn: () => nodeApi.previewConfig(node.id).then((r) => r.data.data!),
    retry: false,
  })
  const { data: nodeKeys } = useQuery({
    queryKey: ['nodeKeys', node.id],
    queryFn: () => nodeApi.getKeys(node.id).then((r) => r.data.data ?? []),
    retry: false,
  })

  useEffect(() => {
    if (!copied) return
    const timer = setTimeout(() => setCopied(false), 1800)
    return () => clearTimeout(timer)
  }, [copied])

  const handleCopy = async () => {
    if (!data?.config) return
    const ok = await copyText(data.config)
    if (ok) {
      setCopied(true)
    } else {
      pushToast({
        title: '复制失败',
        description: '请手动选中预览内容并按 Ctrl+C 复制',
        variant: 'warning',
      })
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={`预览生成配置 · ${node.name}`}
      size="xl"
      footer={
        <Btn variant="secondary" onClick={onClose}>
          关闭
        </Btn>
      }
    >
      <div className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex flex-wrap gap-2">
            {(
              [
                ['config', '完整配置'],
                ['source', '配置来源'],
                ['comparison', '变更对比'],
              ] as const
            ).map(([id, label]) => (
              <Btn key={id} variant={tab === id ? 'primary' : 'secondary'} onClick={() => setTab(id)}>
                {label}
              </Btn>
            ))}
          </div>
          <Btn variant="secondary" onClick={() => void handleCopy()} disabled={!data?.config}>
            <Copy className="h-4 w-4" />
            {copied ? '已复制' : '复制脱敏配置'}
          </Btn>
        </div>
        {tab === 'config' && (
          <div className="space-y-2">
            <p className="text-sm leading-6 text-soft">面板按已保存的协议、用户和路由生成期望配置。敏感字段已脱敏。</p>
            {nodeKeys && availableProfiles && (
              <ProtocolSummary nodeKeys={nodeKeys} activeProfiles={availableProfiles} />
            )}
          </div>
        )}
        {tab === 'source' && (
          <div className="space-y-3 text-sm">
            {source.error ? (
              <Btn variant="secondary" onClick={() => void source.refetch()}>
                来源加载失败，重试
              </Btn>
            ) : source.data ? (
              <>
                <p>
                  {overview.data?.nodes.find((n) => n.id === node.id) &&
                    routingLabel(
                      overview.data.nodes.find((n) => n.id === node.id)!,
                      overview.data.templates,
                    )}
                </p>
                {source.data.routing.mode === 'legacy' && (
                  <p className="text-sm text-soft">{basicRoutingDescription}</p>
                )}
                {Object.entries(source.data.routing.sources)
                  .filter(([key]) => key !== 'schema_version')
                  .map(([key, value]) => (
                    <p key={key}>
                      {(
                        {
                          enable_ipv6_outbound: '旧版 IPv6 开关',
                          ipv4_force_domains: '强制 IPv4 域名',
                          ipv6_force_domains: 'IPv6 域名列表',
                          custom_rules: '自定义规则',
                          ipv6_failure_policy: 'IPv6 故障策略',
                        } as Record<string, string>
                      )[key] ?? key}
                      ：{value}
                    </p>
                  ))}
              </>
            ) : (
              <p>加载来源…</p>
            )}
          </div>
        )}
        {tab === 'comparison' && (
          <div className="space-y-3 text-sm">
            {state.error || source.error ? (
              <Btn variant="secondary" onClick={() => {
                if (state.error) void state.refetch()
                if (source.error) void source.refetch()
              }}>
                确认状态加载失败，重试
              </Btn>
            ) : state.data && source.data ? (
              <>
                <p>
                  {configConfirmed(state.data, source.data.desired_hash)
                    ? '期望配置与最近确认的磁盘、运行版本一致。'
                    : '期望配置与最近记录不同，或仍待确认。'}
                </p>
                <p className="text-soft">
                  对比依据是最近确认的磁盘和运行记录。系统未保留历史配置快照，无法逐行比较。
                </p>
                {state.data.last_error && (
                  <p className="text-[var(--danger)]">{state.data.last_error}</p>
                )}
              </>
            ) : (
              <p>读取确认记录…</p>
            )}
          </div>
        )}
        {tab === 'config' && isLoading && (
          <p className="py-8 text-center text-sm text-soft">生成中…</p>
        )}
        {tab === 'config' && error && (
          <div role="alert" className="rounded-lg border border-[var(--danger-border)] bg-[var(--danger-soft)] p-4">
            <p className="text-sm font-medium text-[var(--danger)]">生成失败</p>
            <p className="mt-1 wrap-anywhere text-sm text-[var(--danger)]">{(error as Error).message}</p>
          </div>
        )}
        {data && tab === 'config' && (
          <>
            {(data.warnings ?? []).length > 0 && (
              <div className="rounded-lg border border-[var(--warning-border)] bg-[var(--warning-soft)] p-4 text-sm text-[var(--warning)]">
                <p className="font-semibold">
                  以下协议生成失败，其余配置可预览
                </p>
                <ul className="mt-1 list-disc space-y-1 pl-5">
                  {data.warnings.map((w, i) => <li key={i} className="wrap-anywhere">{w}</li>)}
                </ul>
              </div>
            )}
            <pre className="max-h-[68vh] overflow-auto rounded-lg border border-[var(--border)] bg-[var(--code-bg)] p-4 font-mono text-[13px] leading-5 text-[var(--code-text)] whitespace-pre">
              {data.config}
            </pre>
          </>
        )}
      </div>
    </Modal>
  )
}

function ProtocolSummary({
  nodeKeys,
  activeProfiles,
}: {
  nodeKeys: NodeKey[]
  activeProfiles: InboundProfile[]
}) {
  const profileMap = new Map(activeProfiles.map((profile) => [profile.id, profile]))
  const matchedProfiles = nodeKeys
    .map((key) => profileMap.get(key.profile_id))
    .filter((item): item is InboundProfile => Boolean(item))

  if (matchedProfiles.length === 0) {
    return <p className="text-sm text-soft">未绑定已启用的协议模板，仅生成系统基础入站。</p>
  }

  return (
    <div className="flex flex-wrap items-center gap-2 text-sm text-soft">
      <span>绑定协议（{matchedProfiles.length}）</span>
      {matchedProfiles.map((profile) => (
        <Badge key={profile.id} label={profile.name} variant="blue" />
      ))}
    </div>
  )
}
