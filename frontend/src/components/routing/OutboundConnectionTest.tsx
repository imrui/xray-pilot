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
        title="从本节点使用已下发的出站配置访问探测地址"
        disabled={dirty}
        loading={test.isPending}
        onClick={() => test.mutate()}
      >
        {test.isPending ? '测试中…' : '测试连接'}
      </Btn>
      <div role="status" className="text-soft">
        {dirty ? '请先保存或放弃当前修改。' : test.isPending ? '正在从本节点通过该出站访问探测地址…' : test.isError ? (
          <span className="text-[var(--danger)]">测试未完成 · {test.error.message}</span>
        ) : result ? (
          <>
            <span className={result.ok ? 'text-[var(--success)]' : 'text-[var(--danger)]'}>
              {result.ok ? '上次测试成功' : '上次测试失败'}
            </span>
            {(result.ok || result.elapsed_ms > 0) && ` · 耗时 ${result.elapsed_ms} ms`}
            {' · '}
            <time dateTime={result.checked_at}>{new Date(result.checked_at).toLocaleString()}</time>
            {!result.ok && result.message && <p>{result.message}</p>}
          </>
        ) : '使用本节点已下发的出站配置访问探测地址。'}
      </div>
    </div>
  )
}
