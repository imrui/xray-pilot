import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Drawer } from '@/components/ui/Drawer'
import { Btn, Field, SelectField } from '@/components/ui/Form'
import { Pagination } from '@/components/ui/Table'
import { useConfirm } from '@/lib/confirm-context'
import { useAuthStore } from '@/store/auth'
import { getBinding, invalidateRouting, routingGet, useRoutingOverview } from '@/lib/routing'
import { jobStatus } from '@/lib/routing-model'
import request from '@/lib/axios'
import { requestID } from '@/lib/request-id'
import type { PageResult, SyncLog } from '@/types'

type Job = {
  id: number
  node_id: number
  status: string
  error: string
  actor: string
  created_at: string
}
import type { SyncSubmission } from '@/lib/node-sync'

export function SyncHistory({
  nodeID,
  submitted = [],
  onClose,
}: {
  nodeID?: number
  submitted?: SyncSubmission[]
  onClose: () => void
}) {
  const qc = useQueryClient()
  const overview = useRoutingOverview()
  const [node, setNode] = useState(nodeID ?? 0)
  const [tab, setTab] = useState<'jobs' | 'logs'>('jobs')
  const [page, setPage] = useState(1)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const query = useQuery({
    queryKey: ['routing-jobs', tab, node, page],
    queryFn: () =>
      routingGet<PageResult<Job | SyncLog>>(
        `/${tab === 'jobs' ? 'config-deployments' : 'node-sync-logs'}?page=${page}&page_size=20${node ? `&node_id=${node}` : ''}`
      ),
    refetchInterval: 3000,
  })
  const action = async (job: Job, reconcile: boolean) => {
    setBusy(true)
    setMessage('')
    try {
      await request.post(
        `/config-deployments/${job.id}/${reconcile ? 'reconcile' : 'retry'}`,
        {},
        { timeout: 60000, headers: { 'Idempotency-Key': requestID() } }
      )
      setMessage(reconcile ? '结果已核对，请查看最新状态。' : '已按最新期望配置创建重试任务。')
      await invalidateRouting(qc)
    } catch (e) {
      setMessage((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Drawer
      open
      width="xl"
      onClose={onClose}
      title="节点同步记录"
      description="同步包含协议、用户和路由的完整配置。关闭窗口不会取消后台任务。"
      footer={
        <Btn variant="secondary" onClick={onClose}>
          关闭
        </Btn>
      }
    >
      <div className="mb-4 flex flex-wrap gap-2">
        <Btn
          variant={tab === 'jobs' ? 'primary' : 'secondary'}
          onClick={() => {
            setTab('jobs')
            setPage(1)
          }}
        >
          同步任务
        </Btn>
        <Btn
          variant={tab === 'logs' ? 'primary' : 'secondary'}
          onClick={() => {
            setTab('logs')
            setPage(1)
          }}
        >
          同步日志（含自动同步）
        </Btn>
      </div>
      <SelectField
        label="节点"
        value={node}
        onChange={(v) => {
          setNode(Number(v))
          setPage(1)
        }}
        options={[
          { value: 0, label: '全部节点' },
          ...(overview.data?.nodes ?? []).map((n) => ({ value: n.id, label: n.name })),
        ]}
      />
      {submitted.length > 0 && (
        <div className="my-4 space-y-2 rounded border border-[var(--border)] p-3 text-sm">
          <p>
            本次提交：{submitted.filter((x) => x.jobID).length} 个已创建任务，
            {submitted.filter((x) => x.error).length} 个未提交。
          </p>
          {submitted.map((r) => (
            <p key={r.nodeID} className={r.error ? 'text-[var(--danger)]' : 'text-soft'}>
              {overview.data?.nodes.find((n) => n.id === r.nodeID)?.name ?? `节点 #${r.nodeID}`}：
              {r.error ?? `任务 #${r.jobID}`}
            </p>
          ))}
        </div>
      )}
      {message && (
        <p role="status" className="my-3 text-sm">
          {message}
        </p>
      )}
      {query.error && (
        <div role="alert">
          记录加载失败。
          <Btn variant="secondary" onClick={() => void query.refetch()}>
            重试
          </Btn>
        </div>
      )}
      {query.isLoading && <p className="py-6 text-soft">加载记录…</p>}
      <div className="mt-4 space-y-3">
        {query.data?.list?.map((row) =>
          tab === 'jobs'
            ? (() => {
                const j = row as Job
                return (
                  <div
                    key={j.id}
                    className="space-y-2 rounded-md border border-[var(--border)] p-4 text-sm"
                  >
                    <div className="flex flex-wrap justify-between gap-2">
                      <strong>
                        #{j.id} ·{' '}
                        {overview.data?.nodes.find((n) => n.id === j.node_id)?.name ??
                          `节点 #${j.node_id}`}
                      </strong>
                      <span>{jobStatus[j.status] ?? j.status}</span>
                    </div>
                    <p className="text-xs text-soft">
                      {new Date(j.created_at).toLocaleString()} · {j.actor}
                    </p>
                    {j.error && <p className="break-words text-[var(--danger)]">{j.error}</p>}
                    {j.status === 'rolled_back' && (
                      <p className="text-xs text-soft">
                        已恢复服务，目标仍待同步；调整路由后可重试。
                      </p>
                    )}
                    <div className="flex gap-2">
                      {j.status === 'unknown' && (
                        <Btn
                          variant="secondary"
                          loading={busy}
                          onClick={() => void action(j, true)}
                        >
                          核对结果
                        </Btn>
                      )}
                      {['failed', 'rolled_back', 'rollback_failed'].includes(j.status) && (
                        <Btn
                          variant="secondary"
                          loading={busy}
                          onClick={() => void action(j, false)}
                        >
                          用最新配置重试
                        </Btn>
                      )}
                    </div>
                  </div>
                )
              })()
            : (() => {
                const log = row as SyncLog
                return (
                  <div
                    key={log.id}
                    className="space-y-2 rounded-md border border-[var(--border)] p-4 text-sm"
                  >
                    <strong>
                      {log.target} · {log.success ? '同步成功' : '同步未成功'}
                    </strong>
                    <p className="text-xs text-soft">
                      {new Date(log.created_at).toLocaleString()} · {log.actor}
                    </p>
                    <p className="break-words">{log.message}</p>
                  </div>
                )
              })()
        )}
      </div>
      {query.data && !query.data.list?.length && (
        <p className="py-6 text-sm text-soft">暂无同步记录。</p>
      )}
      <Pagination page={page} pageSize={20} total={query.data?.total ?? 0} onChange={setPage} />
    </Drawer>
  )
}

export function NodeDiagnostics({
  id,
  name,
  onClose,
}: {
  id: number
  name: string
  onClose: () => void
}) {
  const confirm = useConfirm()
  const qc = useQueryClient()
  const superAdmin = useAuthStore((s) => s.role) === 'super_admin'
  const [host, setHost] = useState('')
  const [busy, setBusy] = useState(false)
  const [output, setOutput] = useState('')
  const state = useQuery({
    queryKey: ['config-state', id],
    queryFn: () => routingGet<Record<string, unknown>>(`/nodes/${id}/config-state`),
  })
  const run = async (kind: 'sni-diagnostics' | 'preflight' | 'restore-policy') => {
    if (
      kind === 'restore-policy' &&
      !(await confirm({
        title: '恢复最近健康路由策略？',
        description: '将改变节点绑定并同步配置，继续使用当前有效用户和密钥。',
        confirmText: '恢复策略',
        cancelText: '取消',
        tone: 'danger',
      }))
    )
      return
    setBusy(true)
    setOutput('')
    try {
      const body =
        kind === 'sni-diagnostics'
          ? { host: host.trim() }
          : kind === 'restore-policy'
            ? { version: (await getBinding('node', id)).version, request_key: requestID() }
            : {}
      const res = await request.post(`/nodes/${id}/${kind}`, body, { timeout: 240000 })
      setOutput(
        kind === 'preflight'
          ? '预检通过，未替换正式配置。'
          : kind === 'restore-policy'
            ? '已创建恢复任务，请在同步记录查看结果。'
            : typeof res.data.data === 'string'
              ? res.data.data
              : JSON.stringify(res.data.data, null, 2)
      )
      await invalidateRouting(qc)
    } catch (e) {
      setOutput((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Drawer
      open
      width="lg"
      onClose={onClose}
      title={`节点诊断与恢复 · ${name}`}
      footer={
        <Btn variant="secondary" onClick={onClose}>
          关闭
        </Btn>
      }
    >
      <div className="space-y-4">
        <Field
          label="公开 SNI 域名"
          value={host}
          onChange={(e) => setHost(e.target.value)}
          placeholder="www.example.com"
        />
        <p className="text-xs text-soft">从节点检查 TLS 和 HTTP；结果不替代 Reality 握手验证。</p>
        <div className="flex flex-wrap gap-2">
          <Btn
            variant="secondary"
            loading={busy}
            disabled={!host.trim()}
            onClick={() => void run('sni-diagnostics')}
          >
            诊断 SNI
          </Btn>
          <Btn variant="secondary" loading={busy} onClick={() => void run('preflight')}>
            配置预检
          </Btn>
          {superAdmin && (
            <Btn variant="secondary" loading={busy} onClick={() => void run('restore-policy')}>
              恢复最近健康策略
            </Btn>
          )}
        </div>
        {output && (
          <pre
            role="status"
            className="max-h-80 overflow-auto whitespace-pre-wrap break-words text-xs"
          >
            {output}
          </pre>
        )}
        <details>
          <summary className="cursor-pointer text-sm">磁盘与运行确认状态</summary>
          <p className="my-2 text-xs text-soft">
            运行哈希是最近应用确认记录，不是实时读取的运行配置。
          </p>
          {state.error ? (
            <Btn variant="secondary" onClick={() => void state.refetch()}>
              加载失败，重试
            </Btn>
          ) : (
            <pre className="max-h-80 overflow-auto text-xs">
              {JSON.stringify(state.data, null, 2)}
            </pre>
          )}
        </details>
      </div>
    </Drawer>
  )
}
