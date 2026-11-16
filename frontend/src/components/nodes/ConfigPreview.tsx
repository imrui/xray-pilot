import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Copy } from 'lucide-react'
import { nodeApi, profileApi } from '@/lib/api'
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
    queryFn: () =>
      profileApi
        .list({ page: 1, page_size: 200 })
        .then((r) => r.data.data?.list.filter((p) => p.active) ?? []),
    enabled: activeProfiles === undefined,
  })
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
        <div className="flex items-center justify-between gap-3">
          <div className="space-y-1">
            <p className="text-xs text-soft">
              预览已保存设置生成的完整期望配置，包含协议、用户和路由；敏感字段已脱敏。不是节点当前配置文件。
            </p>
            <ProtocolSummary
              nodeKeys={nodeKeys ?? []}
              activeProfiles={activeProfiles ?? profiles.data ?? []}
            />
          </div>
          <Btn variant="secondary" onClick={() => void handleCopy()} disabled={!data?.config}>
            <Copy className="h-4 w-4" />
            {copied ? '已复制' : '快速复制'}
          </Btn>
        </div>
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
                  <p className="text-xs text-soft">{basicRoutingDescription}</p>
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
            {state.error ? (
              <Btn variant="secondary" onClick={() => void state.refetch()}>
                确认状态加载失败，重试
              </Btn>
            ) : state.data && source.data ? (
              <>
                <p>
                  {configConfirmed(state.data, source.data.desired_hash)
                    ? '期望配置与最近确认的磁盘、运行版本一致。'
                    : '当前期望配置尚未完整确认生效，或与最近确认记录不同。'}
                </p>
                <p className="text-soft">
                  系统目前没有完整历史配置快照，无法提供可靠的逐行差异。上方「完整配置」是本次准备同步的内容，不能当作远端当前文件。
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
        {error && (
          <div className="rounded-2xl border border-[var(--danger-border)] bg-[var(--danger-soft)] p-4">
            <p className="text-sm font-medium text-[var(--danger)]">生成失败</p>
            <p className="mt-1 text-xs text-[var(--danger)]">{(error as Error).message}</p>
          </div>
        )}
        {data && tab === 'config' && (
          <>
            {(data.warnings ?? []).length > 0 && (
              <div className="rounded-2xl border border-[var(--warning-border)] bg-[var(--warning-soft)] p-4">
                <p className="mb-2 text-xs font-medium text-[var(--warning)]">
                  部分协议生成失败，不影响其他协议：
                </p>
                {data.warnings.map((w, i) => (
                  <p key={i} className="text-xs text-[var(--warning)]">
                    {w}
                  </p>
                ))}
              </div>
            )}
            <pre className="max-h-[68vh] overflow-auto rounded-lg border border-[var(--border)] bg-[var(--code-bg)] p-4 font-mono text-xs leading-5 text-[var(--code-text)] whitespace-pre">
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
    return <p className="text-xs text-soft">当前节点未绑定任何协议，预览只会包含系统基础入站。</p>
  }

  return (
    <div className="flex flex-wrap items-center gap-2 text-xs text-soft">
      <span>当前绑定协议 {matchedProfiles.length} 个：</span>
      {matchedProfiles.map((profile) => (
        <Badge key={profile.id} label={profile.name} variant="blue" />
      ))}
    </div>
  )
}
