import { useQuery } from '@tanstack/react-query'
import { Btn } from '@/components/ui/Form'
import { nodeApi } from '@/lib/api'
import { routingLabel } from '@/lib/routing-model'
import type { RoutingNode, RoutingTemplate } from '@/lib/routing-model'

export function NodeRoutingPreview({
  node,
  templates,
  onPreview,
}: {
  node: RoutingNode
  templates: RoutingTemplate[]
  onPreview: () => void
}) {
  const query = useQuery({
    queryKey: ['previewConfig', node.id],
    queryFn: () => nodeApi.previewConfig(node.id).then((r) => r.data.data!),
    retry: false,
  })
  const config = query.data ? JSON.parse(query.data.config) : null
  const policy = node.routing?.policy
  return (
    <section
      aria-label={`已保存路由 · ${node.name}`}
      className="space-y-3 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] p-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h4 className="text-sm font-semibold">当前节点 · {node.name}</h4>
        <Btn variant="secondary" onClick={onPreview}>
          查看完整配置
        </Btn>
      </div>
      <p className="text-sm">{routingLabel(node, templates)}</p>
      <p className="text-xs text-soft">
        已保存的期望配置，尚未保存的版本选择不包含在此预览中；实际生效状态以节点同步结果为准。
      </p>
      {policy && (
        <div className="space-y-1 text-xs text-soft">
          <p>
            {policy.schema_version === 2
              ? '未命中规则：默认直连，不强制地址族'
              : '旧版策略：未命中仍走 IPv4，升级模板版本后改为默认直连'}
          </p>
          <p>
            强制 IPv4 {(policy.ipv4_force_domains ?? []).length} 条 ·{' '}
            {policy.schema_version === 2 ? '强制 IPv6' : '旧版 IPv6 列表'}{' '}
            {(policy.ipv6_force_domains ?? []).length} 条 · 自定义规则{' '}
            {(policy.custom_rules ?? []).length} 条
          </p>
          {Object.entries(node.routing?.sources ?? {}).filter(([, source]) => source === '节点覆盖')
            .length > 0 && <p>包含本节点单独设置的覆盖项；更换模板时仍会保留。</p>}
        </div>
      )}
      {query.isLoading && <p className="text-sm text-soft">生成路由预览…</p>}
      {query.error && (
        <div role="alert" className="space-y-2 text-sm text-[var(--danger)]">
          <p>路由预览失败：{(query.error as Error).message}</p>
          <Btn variant="secondary" onClick={() => void query.refetch()}>
            重新加载预览
          </Btn>
        </div>
      )}
      {config && !query.error && (
        <details open>
          <summary className="cursor-pointer text-xs font-medium">路由与出站 JSON</summary>
          <pre className="mt-2 max-h-64 overflow-auto rounded border border-[var(--border)] bg-[var(--code-bg)] p-3 font-mono text-xs leading-5 text-[var(--code-text)]">
            {JSON.stringify({ outbounds: config.outbounds, routing: config.routing }, null, 2)}
          </pre>
        </details>
      )}
      {(query.data?.warnings ?? []).map((warning, index) => (
        <p key={index} className="text-xs text-[var(--warning)]">
          {warning}
        </p>
      ))}
    </section>
  )
}
