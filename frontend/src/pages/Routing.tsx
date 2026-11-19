import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { HelpPopover } from '@/components/ui/HelpPopover'
import { Eye, PencilLine, Plus } from 'lucide-react'
import { useAuthStore } from '@/store/auth'
import { PageShell } from '@/components/ui/Page'
import { Btn } from '@/components/ui/Form'
import { ListToolbar, FilterChip } from '@/components/ui/ListToolbar'
import { Table, Pagination } from '@/components/ui/Table'
import { ActionMenu } from '@/components/ui/ActionMenu'
import { useConfirm } from '@/lib/confirm-context'
import { RoutingBindingDrawer } from '@/components/routing/BindingEditor'
import { TemplateEditor, TemplateHistory, TemplateNodes } from '@/components/routing/Templates'
import { emptyPolicy, readPolicy, requiresIPv6 } from '@/lib/routing-model'
import type { RoutingTemplate } from '@/lib/routing-model'
import { invalidateRouting, useRoutingOverview } from '@/lib/routing'
import request from '@/lib/axios'

function DraftSummary({ draft }: { draft: string }) {
  const policy = readPolicy(draft)
  const legacy = (policy.schema_version ?? 1) < 2
  const ipv4 = policy.ipv4_force_domains.length
  const ipv6 = policy.ipv6_force_domains.length
  const rules = policy.custom_rules.length
  const outbounds = policy.custom_outbounds?.length ?? 0
  const balancers = policy.balancers?.length ?? 0
  const parts = [
    legacy && '默认 IPv4（旧版）',
    ipv4 > 0 && `IPv4 域名 ${ipv4}`,
    ipv6 > 0 && `${legacy ? '旧版 IPv6 列表' : 'IPv6 域名'} ${ipv6}`,
    rules > 0 && `${rules} 条规则`,
    outbounds > 0 && `${outbounds} 个出站`,
    balancers > 0 && `${balancers} 个负载均衡`,
    policy.dns?.enabled && '自定义 DNS',
  ].filter(Boolean)
  return (
    <div className="space-y-1 whitespace-normal text-sm leading-6">
      <p>{parts.length ? parts.join(' · ') : '默认直连'}</p>
      {requiresIPv6(policy) && <p className="text-[var(--warning)]">需要可用 IPv6 出口</p>}
    </div>
  )
}

export default function Routing() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const superAdmin = useAuthStore((s) => s.role) === 'super_admin'
  const query = useRoutingOverview()
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState('all')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [edit, setEdit] = useState<RoutingTemplate | null>(null)
  const [history, setHistory] = useState<RoutingTemplate | null>(null)
  const [nodes, setNodes] = useState<RoutingTemplate | null>(null)
  const [global, setGlobal] = useState(false)
  const [error, setError] = useState('')
  const rows = (query.data?.templates ?? []).filter(
    (t) =>
      `${t.name} ${t.description}`.toLowerCase().includes(search.trim().toLowerCase()) &&
      (filter === 'all' || (filter === 'draft' ? t.draft_changed : t.latest_revision > 0)),
  )
  const remove = async (t: RoutingTemplate) => {
    if (
      !(await confirm({
        title: `删除未发布模板「${t.name}」？`,
        description: '仅未发布且未被引用的模板可以删除。',
        confirmText: '删除模板',
        cancelText: '取消',
        tone: 'danger',
      }))
    )
      return
    try {
      await request.delete(`/routing-templates/${t.id}`)
      await invalidateRouting(qc)
    } catch (e) {
      setError((e as Error).message)
    }
  }
  return (
    <PageShell>
      <ListToolbar
        searchValue={search}
        searchPlaceholder="搜索路由模板名称或说明"
        onSearchChange={(s) => {
          setSearch(s)
          setPage(1)
        }}
        filters={
          <>
            {[
              ['all', '全部'],
              ['draft', '有草稿'],
              ['published', '已有版本'],
            ].map(([id, label]) => (
              <FilterChip
                key={id}
                active={filter === id}
                onClick={() => {
                  setFilter(id)
                  setPage(1)
                }}
              >
                {label}
              </FilterChip>
            ))}
          </>
        }
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <HelpPopover title="路由配置说明">
              <p>预设仅供选用。要应用模板，请先绑定已发布的版本，再到节点列表预览并同步配置。</p>
              <p>节点优先使用单独指定的版本，其次使用分组配置，最后继承全局默认。</p>
            </HelpPopover>
            <Btn variant="secondary" onClick={() => setGlobal(true)}>
              全局默认
            </Btn>
            {superAdmin && (
              <Btn
                onClick={() =>
                  setEdit({
                    id: 0,
                    name: '',
                    description: '',
                    draft: JSON.stringify(emptyPolicy),
                    version: 0,
                    latest_revision: 0,
                    draft_changed: true,
                    binding_count: 0,
                    node_count: 0,
                  })
                }
              >
                <Plus className="h-4 w-4" />
                新增路由模板
              </Btn>
            )}
          </div>
        }
      />
      {(error || query.error) && (
        <div role="alert" className="text-sm text-[var(--danger)]">
          {error || '路由模板加载失败。'}{' '}
          <Btn
            variant="secondary"
            onClick={() => {
              setError('')
              void query.refetch()
            }}
          >
            重试
          </Btn>
        </div>
      )}
      {!query.error && (
        <div className="[&_table]:table-fixed [&_th]:text-sm [&_th]:tracking-normal [&_th]:text-soft [&_th:first-child]:w-[30%] [&_th:last-child]:text-right [&_td]:py-4 [&_span.uppercase]:text-sm [&_span.uppercase]:tracking-normal">
          <Table
            data={rows.slice((page - 1) * pageSize, page * pageSize)}
            loading={query.isLoading}
            columns={[
              {
                key: 'name',
                label: '模板',
                render: (t) => (
                  <div className="min-w-0 space-y-1">
                    <div className="flex flex-wrap items-baseline justify-end gap-x-2 md:justify-start">
                      <button
                        className="wrap-anywhere whitespace-normal text-right text-base font-semibold leading-6 text-[var(--text)] hover:text-[var(--accent)] focus-visible:outline-[var(--accent)] md:text-left"
                        onClick={() => setEdit(t)}
                      >
                        {t.name}
                      </button>
                      {t.preset_key && <span className="text-sm font-normal text-soft">预设</span>}
                    </div>
                    {t.description && (
                      <p className="line-clamp-1 wrap-anywhere whitespace-normal text-sm font-normal leading-6 text-soft" title={t.description}>
                        {t.description}
                      </p>
                    )}
                  </div>
                ),
              },
              {
                key: 'rules',
                label: '草稿策略',
                render: (t) => <DraftSummary draft={t.draft} />,
              },
              {
                key: 'usage',
                label: '发布与使用',
                render: (t) => (
                  <div className="space-y-1 whitespace-normal text-sm leading-6">
                    <p>
                      {t.latest_revision ? `已发布 v${t.latest_revision}` : '未发布'}
                      {t.latest_revision > 0 && t.draft_changed && (
                        <span className="ml-2 inline-block text-[var(--warning)]">草稿有修改</span>
                      )}
                    </p>
                    <button
                      className="text-right font-normal text-soft hover:text-[var(--accent)] hover:underline md:text-left"
                      onClick={() => setNodes(t)}
                    >
                      {t.node_count ? `${t.node_count} 个节点` : '暂无节点使用'}
                      {t.binding_count > 0 && ` · ${t.binding_count} 条直接绑定`}
                    </button>
                  </div>
                ),
              },
              {
                key: 'actions',
                label: '操作',
                render: (t) => (
                  <div className="flex flex-wrap items-center justify-end gap-2">
                    <Btn
                      variant="secondary"
                      className="whitespace-nowrap px-3"
                      onClick={() => setEdit(t)}
                    >
                      {superAdmin ? <PencilLine className="size-4" /> : <Eye className="size-4" />}
                      {superAdmin ? '编辑' : '查看'}
                    </Btn>
                    <Btn
                      variant="secondary"
                      className="whitespace-nowrap px-3"
                      disabled={!t.latest_revision}
                      onClick={() => setNodes(t)}
                    >
                      配置节点
                    </Btn>
                    <ActionMenu
                      items={[
                        { label: '版本历史与引用', onSelect: () => setHistory(t) },
                        ...(superAdmin
                          ? [
                              {
                                label: '复制草稿为新模板',
                                onSelect: () =>
                                  setEdit({
                                    id: 0,
                                    name: `${t.name} - 副本`,
                                    description: t.description,
                                    draft: t.draft,
                                    version: 0,
                                    latest_revision: 0,
                                    draft_changed: true,
                                    binding_count: 0,
                                    node_count: 0,
                                  }),
                              },
                            ]
                          : []),
                        ...(superAdmin && !t.latest_revision
                          ? [
                              {
                                label: '删除未发布模板',
                                danger: true,
                                onSelect: () => void remove(t),
                              },
                            ]
                          : []),
                      ]}
                    />
                  </div>
                ),
              },
            ]}
          />
        </div>
      )}
      <Pagination page={page} pageSize={pageSize} total={query.data ? rows.length : undefined} onChange={setPage} onPageSizeChange={setPageSize} />
      {edit && (
        <TemplateEditor
          key={`${edit.id}:${edit.edit_payload ?? edit.draft}`}
          template={edit}
          onClose={() => setEdit(null)}
          onConfigure={(t) => {
            setEdit(null)
            setNodes(t)
          }}
        />
      )}
      {history && (
        <TemplateHistory
          template={history}
          onClose={() => setHistory(null)}
          onEdit={(t) => {
            setHistory(null)
            setEdit(t)
          }}
        />
      )}
      {nodes && <TemplateNodes template={nodes} onClose={() => setNodes(null)} />}
      {global && (
        <RoutingBindingDrawer
          scope="global"
          id={0}
          name="全局默认"
          onClose={() => setGlobal(false)}
        />
      )}
    </PageShell>
  )
}
