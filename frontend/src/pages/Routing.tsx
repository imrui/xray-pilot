import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { PencilLine, Plus } from 'lucide-react'
import { useAuthStore } from '@/store/auth'
import { PageShell } from '@/components/ui/Page'
import { Btn } from '@/components/ui/Form'
import { ListToolbar, FilterChip } from '@/components/ui/ListToolbar'
import { Table, Pagination } from '@/components/ui/Table'
import { Badge } from '@/components/ui/Badge'
import { ActionMenu } from '@/components/ui/ActionMenu'
import { useConfirm } from '@/lib/confirm-context'
import { RoutingBindingDrawer } from '@/components/routing/BindingEditor'
import { TemplateEditor, TemplateHistory, TemplateNodes } from '@/components/routing/Templates'
import { emptyPolicy, readPolicy } from '@/lib/routing-model'
import type { RoutingTemplate } from '@/lib/routing-model'
import { invalidateRouting, useRoutingOverview } from '@/lib/routing'
import request from '@/lib/axios'

export default function Routing() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const superAdmin = useAuthStore((s) => s.role) === 'super_admin'
  const query = useRoutingOverview()
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState('all')
  const [page, setPage] = useState(1)
  const [edit, setEdit] = useState<RoutingTemplate | null>(null)
  const [history, setHistory] = useState<RoutingTemplate | null>(null)
  const [nodes, setNodes] = useState<RoutingTemplate | null>(null)
  const [global, setGlobal] = useState(false)
  const [error, setError] = useState('')
  const rows = (query.data?.templates ?? []).filter(
    (t) =>
      (t.name + t.description).toLowerCase().includes(search.toLowerCase()) &&
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
          <>
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
          </>
        }
      />
      <p className="text-sm text-soft">
        模板定义路由规则；配置节点后，在节点列表预览和同步完整配置。
      </p>
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
        <Table
          data={rows.slice((page - 1) * 10, page * 10)}
          loading={query.isLoading}
          columns={[
            {
              key: 'name',
              label: '名称',
              render: (t) => (
                <div>
                  <button className="font-semibold text-[var(--accent)]" onClick={() => setEdit(t)}>
                    {t.name}
                  </button>
                  <p className="mt-1 max-w-xs whitespace-normal text-xs text-soft">
                    {t.description || '未填写说明'}
                  </p>
                </div>
              ),
            },
            {
              key: 'rules',
              label: '草稿规则摘要',
              render: (t) => {
                const p = readPolicy(t.draft)
                return (
                  <div className="space-y-1 text-xs">
                    <p>
                      强制 IPv4 {p.ipv4_force_domains.length} ·{' '}
                      {p.schema_version === 2 ? '强制 IPv6' : '旧版 IPv6 列表'}{' '}
                      {p.ipv6_force_domains.length}
                    </p>
                    <p>
                      {p.schema_version === 2 ? '默认直连' : '旧版：默认 IPv4'} · 自定义规则{' '}
                      {p.custom_rules.length}
                    </p>
                  </div>
                )
              },
            },
            {
              key: 'latest_revision',
              label: '最新可用版本',
              render: (t) => (t.latest_revision ? `v${t.latest_revision}` : '尚未发布'),
            },
            {
              key: 'usage',
              label: '使用情况',
              render: (t) => (
                <button
                  className="text-left text-xs text-[var(--accent)]"
                  onClick={() => setNodes(t)}
                >
                  {t.node_count} 个节点使用
                  <p className="mt-1 text-soft">{t.binding_count} 条直接绑定</p>
                </button>
              ),
            },
            {
              key: 'draft_changed',
              label: '草稿状态',
              render: (t) => (
                <Badge
                  label={t.draft_changed ? '有未发布修改' : '与最新版本一致'}
                  variant={t.draft_changed ? 'yellow' : 'gray'}
                />
              ),
            },
            {
              key: 'actions',
              label: '操作',
              render: (t) => (
                <div className="flex justify-end gap-2">
                  <Btn variant="secondary" onClick={() => setEdit(t)}>
                    <PencilLine className="h-4 w-4" />
                    {superAdmin ? '编辑' : '查看'}
                  </Btn>
                  <Btn
                    variant="secondary"
                    disabled={!t.latest_revision}
                    onClick={() => setNodes(t)}
                  >
                    配置节点
                  </Btn>
                  <ActionMenu
                    items={[
                      { label: '版本历史与引用', onSelect: () => setHistory(t) },
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
      )}
      <Pagination page={page} pageSize={10} total={rows.length} onChange={setPage} />
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
