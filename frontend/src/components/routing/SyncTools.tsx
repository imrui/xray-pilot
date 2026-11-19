import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Drawer } from '@/components/ui/Drawer'
import { Btn, Field, FieldGroup, SelectField } from '@/components/ui/Form'
import { Pagination } from '@/components/ui/Table'
import { useConfirm } from '@/lib/confirm-context'
import { useAuthStore } from '@/store/auth'
import { getBinding, invalidateRouting, routingGet, useRoutingOverview } from '@/lib/routing'
import { jobStatus } from '@/lib/routing-model'
import request from '@/lib/axios'
import { requestID } from '@/lib/request-id'
import type { SyncSubmission } from '@/lib/node-sync'
import type { PageResult, SyncLog } from '@/types'

type Job = {
  id: number
  node_id: number
  status: string
  error: string
  actor: string
  created_at: string
}

const baselineAdminNotice = '远端配置与纳管基线不一致，请联系超级管理员核对并接管基线'

export function SyncHistory({
  nodeID,
  submitted = [],
  onInspect,
  onClose,
}: {
  nodeID?: number
  submitted?: SyncSubmission[]
  onInspect: (id: number, name: string) => void
  onClose: () => void
}) {
  const qc = useQueryClient()
  const overview = useRoutingOverview()
  const superAdmin = useAuthStore((s) => s.role) === 'super_admin'
  const [node, setNode] = useState(nodeID ?? 0)
  const [tab, setTab] = useState<'jobs' | 'logs'>('jobs')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const selectedError = overview.data?.nodes.find(n => n.id === node)?.last_sync_error
  const selectedConflict = selectedError?.includes('DRIFT_CONFLICT')
  const baselineAction = (id: number) => superAdmin ? (
    <Btn onClick={() => onInspect(id, overview.data?.nodes.find(n => n.id === id)?.name ?? `节点 #${id}`)}>去核对远端基线</Btn>
  ) : <p className="text-sm text-[var(--warning)]">{baselineAdminNotice}</p>
  const query = useQuery({
    queryKey: ['routing-jobs', tab, node, page, pageSize],
    queryFn: () =>
      routingGet<PageResult<Job | SyncLog>>(
        `/${tab === 'jobs' ? 'config-deployments' : 'node-sync-logs'}?page=${page}&page_size=${pageSize}${node ? `&node_id=${node}` : ''}`
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
      onClose={onClose}
      title="节点同步记录"
      footer={
        <>
          <span className="mr-auto self-center text-sm text-soft">关闭后，已提交的任务会继续执行。</span>
          <Btn variant="secondary" onClick={onClose}>关闭</Btn>
        </>
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
      {selectedConflict && <div className="my-3">{baselineAction(node)}</div>}
      {selectedError && !selectedConflict && <p className="my-3 text-sm text-soft">最近失败原因：{selectedError}</p>}
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
                const conflict = j.status === 'failed' && j.error?.includes('DRIFT_CONFLICT')
                return (
                  <div
                    key={j.id}
                    className="space-y-2 rounded-md border border-[var(--border)] p-4 text-sm"
                  >
                    <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                      <strong>
                        #{j.id} ·{' '}
                        {overview.data?.nodes.find((n) => n.id === j.node_id)?.name ??
                          `节点 #${j.node_id}`}
                      </strong>
                      <span className="wrap-anywhere text-[13px] text-soft">{new Date(j.created_at).toLocaleString()} · {j.actor}</span>
                      <span className="ml-auto">{jobStatus[j.status] ?? j.status}</span>
                    </div>
                    {j.error && <p className="break-words text-[var(--danger)]">{j.error}</p>}
                    {j.status === 'rolled_back' && (
                      <p className="text-sm text-soft">
                        服务已恢复。可调整期望配置后重试。
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
                      {conflict && !(selectedConflict && node === j.node_id) && baselineAction(j.node_id)}
                      {!conflict && ['failed', 'rolled_back', 'rollback_failed'].includes(j.status) && (
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
                    <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                      <strong className="wrap-anywhere">{log.target} · {log.success ? '同步成功' : '同步未成功'}</strong>
                      <span className="wrap-anywhere text-[13px] text-soft">{new Date(log.created_at).toLocaleString()} · {log.actor}</span>
                    </div>
                    <p className="break-words">{log.message}</p>
                  </div>
                )
              })()
        )}
      </div>
      {query.data && !query.data.list?.length && (
        <p className="py-6 text-sm text-soft">暂无同步记录。</p>
      )}
      <Pagination page={page} pageSize={pageSize} total={query.data?.total} onChange={setPage} onPageSizeChange={setPageSize} />
    </Drawer>
  )
}

type BaselineInspection = {
  config_path: string
  observed_raw_sha256: string
  observed_hash: string
  recorded_baseline_hash: string
  desired_hash: string
  baseline_matched: boolean
  remote_valid_json: boolean
  remote_bytes: number
  remote_redacted: string
  desired_redacted: string
  desired_error?: string
}

function BaselinePanel({ id, name }: { id: number; name: string }) {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const [inspection, setInspection] = useState<(BaselineInspection & { requestKey: string }) | null>(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const inspect = async () => {
    setBusy(true)
    setInspection(null)
    setMessage('')
    try {
      const res = await request.post<{ data: BaselineInspection }>(`/nodes/${id}/inspect-baseline`, {}, { timeout: 60000 })
      setInspection({ ...res.data.data, requestKey: requestID() })
    } catch (e) {
      setMessage((e as Error).message)
    } finally {
      setBusy(false)
    }
  }
  const adopt = async () => {
    if (!inspection || !(await confirm({
      title: `覆盖「${name}」的远端配置？`,
      description: `将以面板当前有效配置覆盖 ${inspection.config_path}，重启 Xray 并校验运行状态。`,
      confirmText: '确认覆盖并同步',
      tone: 'danger',
    }))) return
    setBusy(true)
    setMessage('')
    try {
      const res = await request.post<{ data: Job[] }>(`/nodes/${id}/adopt-baseline`, {
        observed_raw_sha256: inspection.observed_raw_sha256,
        request_key: inspection.requestKey,
      }, { timeout: 60000 })
      setInspection(null)
      setMessage(`已接管基线，同步任务 #${res.data.data[0].id} 已入队。`)
      await invalidateRouting(qc)
    } catch (e) {
      const error = (e as Error).message
      if (error.includes('重新核对')) setInspection(null)
      setMessage(error)
    } finally {
      setBusy(false)
    }
  }
  return (
    <FieldGroup title="远端配置核对与基线接管">
      <Btn variant="secondary" disabled={busy} onClick={() => void inspect()}>核对远端配置</Btn>
      {inspection && <>
        <p className={inspection.baseline_matched ? 'text-sm font-medium' : 'text-sm font-medium text-[var(--warning)]'}>{inspection.baseline_matched ? '远端配置已通过基线校验。' : '远端配置与纳管基线不一致。'}</p>
        <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-sm">
          {[
            ['远端路径', inspection.config_path],
            ['面板基线哈希', inspection.recorded_baseline_hash || '尚无记录'],
            ['远端配置哈希', inspection.observed_hash],
            ['远端原始 SHA-256', inspection.observed_raw_sha256],
            ['期望配置哈希', inspection.desired_hash || '暂不可用'],
          ].map(([label, value]) => <div key={label} className="contents">
            <dt className="text-soft">{label}</dt><dd className="break-all font-mono text-[13px] leading-5">{value}</dd>
          </div>)}
        </dl>
        {!inspection.remote_valid_json && <p className="text-sm text-[var(--warning)]">远端文件不是合法 JSON（{inspection.remote_bytes} 字节）。</p>}
        {inspection.desired_error && <p role="alert" className="text-sm text-[var(--warning)]">{inspection.desired_error}</p>}
        <details>
          <summary className="cursor-pointer text-sm">对比脱敏配置</summary>
          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            {[['远端配置', inspection.remote_redacted], ['面板期望配置', inspection.desired_redacted || '暂不可用']].map(([label, value]) => (
              <div key={label} className="min-w-0">
                <h5 className="mb-1 text-sm font-medium">{label}</h5>
                <pre className="max-h-72 overflow-auto rounded bg-[var(--code-bg)] p-2 text-[13px] leading-5">{value}</pre>
              </div>
            ))}
          </div>
        </details>
        {!inspection.baseline_matched && <Btn variant="danger" disabled={busy || !inspection.desired_hash} onClick={() => void adopt()}>确认以面板配置覆盖远端</Btn>}
      </>}
      {message && <p role="status" className="text-sm">{message}</p>}
    </FieldGroup>
  )
}

export function NodeDiagnostics({
  id,
  name,
  lastSyncError,
  onClose,
}: {
  id: number
  name: string
  lastSyncError?: string
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
      onClose={onClose}
      title={`节点诊断与恢复 · ${name}`}
      footer={
        <Btn variant="secondary" onClick={onClose}>
          关闭
        </Btn>
      }
    >
      <div className="space-y-4">
        {superAdmin ? <BaselinePanel id={id} name={name} /> :
          (lastSyncError ?? String(state.data?.last_error ?? '')).includes('DRIFT_CONFLICT') &&
          <p className="text-sm text-[var(--warning)]">{baselineAdminNotice}</p>}
        <Field
          label="公开 SNI 域名"
          value={host}
          onChange={(e) => setHost(e.target.value)}
          placeholder="www.example.com"
        />
        <p className="text-sm text-soft">SNI 诊断从节点检查 TLS 和 HTTP。Reality 握手需另行验证。</p>
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
            className="max-h-80 overflow-auto whitespace-pre-wrap break-words text-[13px] leading-5"
          >
            {output}
          </pre>
        )}
        <details>
          <summary className="cursor-pointer text-sm">磁盘与运行确认状态</summary>
          <p className="my-2 text-sm text-soft">
            运行哈希来自最近一次应用配置的确认记录。
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
