import { useState } from 'react'
import { createPortal } from 'react-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { FlaskConical } from 'lucide-react'
import { Btn } from '@/components/ui/Form'
import { Drawer } from '@/components/ui/Drawer'
import { useConfirm } from '@/lib/confirm-context'
import { useAuthStore } from '@/store/auth'
import { BindingFields } from './BindingFields'
import { DraftPreflightModal } from './DraftPreflightModal'
import {
  getBinding,
  invalidateRouting,
  previewBinding,
  previewTemplate,
  saveBinding,
  useRoutingOverview,
} from '@/lib/routing'
import { affectedIDs, nodesLink, normalizeBinding, routingLabel } from '@/lib/routing-model'
import type { BindingImpact, RoutingBinding, RoutingScope } from '@/lib/routing-model'

export function BindingEditor({
  scope,
  id,
  onDirtyChange,
  onBusyChange,
  footerElement,
}: {
  scope: RoutingScope
  id: number
  onDirtyChange?: (dirty: boolean) => void
  onBusyChange?: (busy: boolean) => void
  footerElement?: HTMLElement | null
}) {
  const qc = useQueryClient()
  const overview = useRoutingOverview()
  const query = useQuery({
    queryKey: ['routing-binding', scope, id],
    queryFn: () => getBinding(scope, id),
  })
  const superAdmin = useAuthStore((s) => s.role) === 'super_admin'
  const [draft, setDraft] = useState<RoutingBinding | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [impact, setImpact] = useState<{ result: BindingImpact; saved: boolean } | null>(null)
  const [sandboxOpen, setSandboxOpen] = useState(false)
  const value = draft ?? query.data
  const dirty = !!draft && JSON.stringify(draft) !== JSON.stringify(query.data)
  const editable = superAdmin || scope === 'node'
  const change = (b: RoutingBinding) => {
    setDraft(b)
    setImpact(null)
    setError('')
    onDirtyChange?.(JSON.stringify(b) !== JSON.stringify(query.data))
  }
  const run = async (save: boolean) => {
    if (!value) return
    setBusy(true)
    onBusyChange?.(true)
    setError('')
    setImpact(null)
    try {
      const payload = normalizeBinding(value)
      const result = await (save ? saveBinding(payload) : previewBinding(payload))
      if (save) {
        // Refresh only after a successful write. Conflicts keep the user's draft.
        const next = { ...payload, version: payload.version + 1 }
        qc.setQueryData(['routing-binding', scope, id], next)
        setDraft(null)
        onDirtyChange?.(false)
        await invalidateRouting(qc)
      }
      setImpact({ result, saved: save })
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
      onBusyChange?.(false)
    }
  }
  if (query.isLoading || overview.isLoading)
    return <p className="py-4 text-sm text-soft">加载配置…</p>
  if (!value || query.error || overview.error)
    return (
      <div role="alert" className="space-y-2 text-sm text-[var(--danger)]">
        路由配置加载失败。
        <Btn
          variant="secondary"
          onClick={() => {
            void query.refetch()
            void overview.refetch()
          }}
        >
          重试
        </Btn>
      </div>
    )
  const source = scope === 'node' ? overview.data?.nodes.find((n) => n.id === id) : null
  const ids = impact ? affectedIDs(impact.result) : []
  const canSandboxNode =
    scope === 'node' &&
    superAdmin &&
    value.mode !== 'legacy' &&
    (value.mode === 'inherit' || (value.template_id > 0 && value.revision > 0))
  const actions = editable ? (
    <div className="flex flex-wrap justify-end gap-2">
      {dirty && (
        <Btn
          variant="secondary"
          disabled={busy}
          onClick={() => {
            setDraft(null)
            setImpact(null)
            setError('')
            onDirtyChange?.(false)
          }}
        >
          放弃本节修改
        </Btn>
      )}
      {canSandboxNode && (
        <Btn variant="accent" disabled={busy} onClick={() => setSandboxOpen(true)}>
          <FlaskConical className="h-4 w-4" />
          沙箱预检
        </Btn>
      )}
      <Btn variant="secondary" loading={busy} onClick={() => void run(false)}>
        查看影响范围
      </Btn>
      <Btn loading={busy} disabled={!dirty} onClick={() => void run(true)}>
        保存路由
      </Btn>
    </div>
  ) : null
  return (
    <div className="space-y-4">
      {source && (
        <div className="rounded-md bg-[var(--panel-muted)] p-3 text-sm">
          <div className="text-xs text-soft">已保存的路由来源</div>
          <p className="mt-1">{routingLabel(source, overview.data?.templates ?? [])}</p>
        </div>
      )}
      <fieldset disabled={busy || !editable} className="space-y-4 disabled:opacity-70">
        <BindingFields
          value={value}
          templates={overview.data?.templates ?? []}
          onChange={change}
        />
      </fieldset>
      {scope === 'group' && (
        <p className="text-xs text-soft">路由策略独立保存，影响范围以已保存的分组成员为准。</p>
      )}
      <p className="text-xs text-soft">
        {overview.data?.auto_sync
            ? '自动同步已开启：保存后，有效配置变化的节点将自动同步。'
            : '当前为手动同步：保存后请在节点列表同步配置。'}
      </p>
      {error && (
        <div role="alert" className="space-y-2 text-sm text-[var(--danger)]">
          <p>{error} 当前输入已保留。</p>
          <Btn
            variant="secondary"
            onClick={async () => {
              await query.refetch()
              setError('已重新读取服务器版本。请对照后决定如何修改；当前草稿仍保留。')
            }}
          >
            重新读取服务器版本
          </Btn>
          {draft && (
            <details>
              <summary>查看服务器当前选择</summary>
              <p className="mt-2">
                {query.data?.mode} · 模板 #{query.data?.template_id} v{query.data?.revision}
              </p>
              <Btn
                variant="secondary"
                onClick={() => {
                  setDraft(null)
                  setError('')
                  onDirtyChange?.(false)
                }}
              >
                放弃草稿，使用服务器配置
              </Btn>
            </details>
          )}
        </div>
      )}
      {impact && (
        <div
          role="status"
          className="space-y-2 rounded-md border border-[var(--border)] p-3 text-sm"
        >
          <p>
            {impact.saved ? '已保存' : '预计影响'}：{impact.result.changed_nodes.length} 个节点
            {impact.saved ? (overview.data?.auto_sync ? '将自动同步' : '待同步') : '的有效配置变化'}
            。
          </p>
          {Object.entries(impact.result.conflicts).map(([nodeID, reason]) => (
            <p key={nodeID} className="text-[var(--danger)]">
              {overview.data?.nodes.find((n) => n.id === Number(nodeID))?.name ?? `节点 #${nodeID}`}
              ：{reason}
            </p>
          ))}
          {!!ids.length && impact.saved && (
            <Link to={nodesLink(ids)} className="text-[var(--accent)] underline">
              查看受影响节点
            </Link>
          )}
          {!impact.saved &&
            ids.map((nodeID) => (
              <p key={nodeID}>
                {overview.data?.nodes.find((n) => n.id === nodeID)?.name ?? `节点 #${nodeID}`}
              </p>
            ))}
          {!ids.length && (
            <p className="text-soft">没有需要同步的有效配置变化。</p>
          )}
        </div>
      )}
      {footerElement ? createPortal(actions, footerElement) : actions}
      {sandboxOpen && scope === 'node' && (
        <DraftPreflightModal
          open={sandboxOpen}
          onClose={() => setSandboxOpen(false)}
          nodes={overview.data?.nodes ?? []}
          lockedNodeID={id}
          customOverrides={(() => {
            try {
              return normalizeBinding(value).overrides
            } catch {
              return value.overrides
            }
          })()}
          getDraftPolicy={async () => {
            const normalized = normalizeBinding(value)
            const resolved = await previewTemplate(normalized)
            if (!resolved.policy) {
              throw new Error('未能解析所选模板策略。')
            }
            return resolved.policy
          }}
        />
      )}
    </div>
  )
}

export function RoutingBindingDrawer({
  scope,
  id,
  name,
  onClose,
  onPreview,
}: {
  scope: RoutingScope
  id: number
  name: string
  onClose: () => void
  onPreview?: () => void
}) {
  const confirm = useConfirm()
  const [dirty, setDirty] = useState(false)
  const [busy, setBusy] = useState(false)
  const [footerElement, setFooterElement] = useState<HTMLDivElement | null>(null)
  const close = async () =>
    !dirty ||
    (await confirm({
      title: '放弃未保存的路由修改？',
      description: '已保存的路由配置不会受影响。',
      confirmText: '放弃修改',
      cancelText: '继续编辑',
      tone: 'danger',
    }))
  return (
    <Drawer
      open
      onClose={onClose}
      title={scope === 'global' ? '全局默认路由' : `节点路由配置 · ${name}`}
      width="lg"
      dirty={dirty}
      saving={busy}
      onBeforeClose={close}
      footer={
        <>
          <Btn
            variant="secondary"
            disabled={busy}
            onClick={() =>
              void close().then((ok) => {
                if (ok) onClose()
              })
            }
          >
            关闭
          </Btn>
          {onPreview && (
            <Btn
              variant="secondary"
              disabled={dirty || busy}
              title={dirty ? '请先保存，预览只包含已保存的配置' : undefined}
              onClick={onPreview}
            >
              查看完整配置
            </Btn>
          )}
          <div ref={setFooterElement} />
        </>
      }
    >
      <BindingEditor
        scope={scope}
        id={id}
        onDirtyChange={setDirty}
        onBusyChange={setBusy}
        footerElement={footerElement}
      />
      {dirty && onPreview && (
        <p className="mt-3 text-xs text-soft">请先保存，再预览包含协议、用户和路由的完整配置。</p>
      )}
    </Drawer>
  )
}
