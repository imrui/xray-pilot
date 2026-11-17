import { useState } from 'react'
import { CheckCircle2, Play, Server, ShieldAlert, Sparkles } from 'lucide-react'
import { Modal } from '@/components/ui/Modal'
import { Badge } from '@/components/ui/Badge'
import { Btn, SelectField, Switch } from '@/components/ui/Form'
import { JsonEditor } from '@/components/ui/JsonEditor'
import { preflightDraft } from '@/lib/routing'
import type { PreflightDraftResponse } from '@/lib/routing'
import type { RoutingNode, RoutingPolicy } from '@/lib/routing-model'

export function DraftPreflightModal({
  open,
  onClose,
  nodes,
  lockedNodeID,
  customOverrides,
  getDraftPolicy,
}: {
  open: boolean
  onClose: () => void
  nodes: RoutingNode[]
  lockedNodeID?: number
  customOverrides?: Record<string, unknown> | string | null
  getDraftPolicy: () => Promise<RoutingPolicy | Record<string, unknown>> | RoutingPolicy | Record<string, unknown>
}) {
  const defaultNodeID = lockedNodeID ?? nodes.find((n) => n.active)?.id ?? nodes[0]?.id ?? 0
  const [selectedNodeID, setSelectedNodeID] = useState<number>(defaultNodeID)
  const [useNodeOverrides, setUseNodeOverrides] = useState(true)
  const [running, setRunning] = useState(false)
  const [result, setResult] = useState<PreflightDraftResponse | null>(null)
  const [requestError, setRequestError] = useState('')

  const effectiveNodeID = lockedNodeID ?? selectedNodeID
  const targetNode = nodes.find((n) => n.id === effectiveNodeID)

  const runPreflight = async () => {
    if (!effectiveNodeID) {
      setRequestError('请先选择一台目标节点。')
      return
    }
    setRunning(true)
    setRequestError('')
    try {
      const policy = await getDraftPolicy()
      const res = await preflightDraft(effectiveNodeID, {
        policy,
        use_node_overrides: customOverrides !== undefined ? false : useNodeOverrides,
        custom_overrides: customOverrides !== undefined ? customOverrides : null,
      })
      setResult(res)
    } catch (e) {
      setResult(null)
      setRequestError((e as Error).message)
    } finally {
      setRunning(false)
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      size="lg"
      title={
        lockedNodeID && targetNode
          ? `远端沙箱预检 · ${targetNode.name}`
          : '未发布草稿远端沙箱预检'
      }
      footer={
        <>
          <Btn variant="secondary" disabled={running} onClick={onClose}>
            关闭
          </Btn>
          <Btn
            loading={running}
            disabled={!effectiveNodeID}
            onClick={() => void runPreflight()}
          >
            <Play className="h-4 w-4" />
            运行远端沙箱预检
          </Btn>
        </>
      }
    >
      <div className="space-y-4">
        <div className="rounded-lg border border-[var(--border)] bg-[var(--panel-muted)] p-4 text-xs text-soft">
          <div className="flex items-start gap-2">
            <Sparkles className="mt-0.5 h-4 w-4 shrink-0 text-[var(--accent)]" />
            <p className="leading-5">
              在内存中合成当前草稿与目标节点的激活入站协议、用户列表，并通过 SSH 在远端节点执行只读{' '}
              <code className="font-mono text-[var(--text)]">xray run -test</code>{' '}
              校验（校验真实内核版本及节点上的 <code className="font-mono text-[var(--text)]">geosite.dat / geoip.dat</code> 条目）。
              零数据库写入、不重启线上服务、不触发任何告警。
            </p>
          </div>
        </div>

        <div className="space-y-3 rounded-lg border border-[var(--border)] bg-[var(--panel)] p-4">
          {lockedNodeID ? (
            <div className="flex items-center gap-2 text-sm">
              <Server className="h-4 w-4 text-[var(--accent)]" />
              <span>
                目标节点：<strong>{targetNode?.name ?? `#${lockedNodeID}`}</strong>
                {targetNode?.ip ? ` (${targetNode.ip})` : ''}
              </span>
              <Badge label="叠加当前待保存覆盖" variant="blue" />
            </div>
          ) : nodes.length === 0 ? (
            <p className="text-sm text-[var(--warning)]">当前暂无可用节点，无法执行远端沙箱预检。</p>
          ) : (
            <>
              <SelectField
                label="选择沙箱验证节点"
                value={effectiveNodeID}
                onChange={(v) => {
                  setSelectedNodeID(Number(v))
                  setResult(null)
                  setRequestError('')
                }}
                options={nodes.map((n) => ({
                  value: n.id,
                  label: `${n.name} (${n.ip})${n.active ? '' : ' · 已停用'}`,
                }))}
              />
              <div className="flex items-center justify-between gap-3 pt-1">
                <span className="text-xs text-soft">
                  叠加该节点当前已保存的节点级覆盖（若存在不兼容旧覆盖将自动跳过并提示）
                </span>
                <Switch checked={useNodeOverrides} onChange={setUseNodeOverrides} />
              </div>
            </>
          )}
        </div>

        {requestError && (
          <div
            role="alert"
            className="flex items-start gap-2 rounded-lg border border-[var(--danger-border)] bg-[var(--danger-soft)] p-3.5 text-sm text-[var(--danger)]"
          >
            <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
            <span>{requestError}</span>
          </div>
        )}

        {result && (
          <div className="space-y-4">
            <div
              role="status"
              className={`space-y-2.5 rounded-lg border p-4 ${
                result.valid
                  ? 'border-[var(--success-border)] bg-[var(--success-soft)]'
                  : 'border-[var(--danger-border)] bg-[var(--danger-soft)]'
              }`}
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-2 text-sm font-semibold">
                  {result.valid ? (
                    <>
                      <CheckCircle2 className="h-4 w-4 text-[var(--success)]" />
                      <span className="text-[var(--success)]">远端 Xray 沙箱预检通过</span>
                    </>
                  ) : (
                    <>
                      <ShieldAlert className="h-4 w-4 text-[var(--danger)]" />
                      <span className="text-[var(--danger)]">沙箱预检未通过</span>
                    </>
                  )}
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  {result.xray_version && (
                    <Badge label={`Xray ${result.xray_version}`} variant="blue" />
                  )}
                  <Badge
                    label={result.requires_ipv6 ? '需要本机 IPv6 出口' : '无需本机 IPv6'}
                    variant={result.requires_ipv6 ? 'yellow' : 'gray'}
                  />
                  <Badge label={`${result.elapsed_ms} ms`} variant="gray" />
                </div>
              </div>

              {result.error && (
                <p className="font-mono text-xs leading-5 text-[var(--danger)]">{result.error}</p>
              )}

              {result.warnings?.length > 0 && (
                <ul className="list-disc space-y-1 pl-5 text-xs text-[var(--warning)]">
                  {result.warnings.map((w, i) => (
                    <li key={i}>{w}</li>
                  ))}
                </ul>
              )}
            </div>

            {result.redacted_config && (
              <div className="space-y-2">
                <span className="block text-xs font-semibold text-[var(--text)]">
                  合成完整配置预览（敏感凭据已自动脱敏）
                </span>
                <JsonEditor
                  ariaLabel="沙箱预检合成配置预览"
                  value={result.redacted_config}
                  readOnly
                  minHeight="240px"
                  maxHeight="360px"
                />
              </div>
            )}
          </div>
        )}
      </div>
    </Modal>
  )
}
