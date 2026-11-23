import { useMutation } from '@tanstack/react-query'
import { Btn } from '@/components/ui/Form'
import { testNodeOutbound } from '@/lib/routing'

export function OutboundConnectionTest({ nodeID, tag, dirty }: { nodeID: number; tag: string; dirty: boolean }) {
  const test = useMutation({ mutationFn: () => testNodeOutbound(nodeID, tag), retry: false })
  const result = test.data
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
      <Btn
        type="button"
        variant="secondary"
        className="h-8 px-3 text-xs"
        aria-label={`测试连接 · ${tag}`}
        title="使用已下发的出站配置测试节点间 TCP 握手和经出站的 HTTPS 请求"
        disabled={dirty}
        loading={test.isPending}
        onClick={() => test.mutate()}
      >
        {test.isPending ? '测试中…' : '测试连接'}
      </Btn>
      <div role="status" className="min-w-0 space-y-1 text-soft">
        {dirty ? '请先保存或放弃当前修改。' : test.isPending ? '正在测试节点间连接和 HTTPS 请求…' : test.isError ? (
          <span className="text-[var(--danger)]">测试未完成 · {test.error.message}</span>
        ) : result ? (
          <>
            <dl className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
              <div className="flex flex-wrap items-baseline gap-x-1">
                <dt title="从本节点向落地入口发起 TCP 连接，按握手耗时估算往返延时">节点间 RTT（TCP 握手估算）</dt>
                <dd className={result.tcp_rtt_ms != null ? 'text-[var(--success)]' : undefined}>
                  {result.tcp_rtt_ms != null ? `${result.tcp_rtt_ms} ms` : result.tcp_rtt_error || '无测量结果'}
                </dd>
              </div>
              <div className="flex flex-wrap items-baseline gap-x-1">
                <dt title="从本节点经选中的出站完成 HTTPS 探测请求的耗时">HTTPS 总耗时</dt>
                <dd className={result.ok ? 'text-[var(--success)]' : 'text-[var(--danger)]'}>
                  {result.ok ? '成功' : '未通过'}
                  {(result.ok || result.elapsed_ms > 0) && ` · ${result.elapsed_ms} ms`}
                  {!result.ok && result.message && ` · ${result.message}`}
                </dd>
              </div>
            </dl>
            <p>上次测试 <time dateTime={result.checked_at}>{new Date(result.checked_at).toLocaleString()}</time></p>
          </>
        ) : '测试到落地入口的节点间 RTT，以及经该出站的 HTTPS 总耗时。'}
      </div>
    </div>
  )
}
