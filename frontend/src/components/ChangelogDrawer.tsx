import { type ReactNode, Fragment, useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { systemApi } from '@/lib/api'
import { Drawer } from '@/components/ui/Drawer'
import { Badge } from '@/components/ui/Badge'
import { cn } from '@/lib/utils'

// ChangelogDrawer 更新日志抽屉：左侧版本导航 + 右侧日志内容（按版本降序）。
// 内容随二进制版本固定，staleTime 设为 Infinity 避免会话内重复请求。
export function ChangelogDrawer({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['release-notes'],
    queryFn: () => systemApi.getReleaseNotes().then((r) => r.data.data ?? []),
    enabled: open,
    staleTime: Infinity,
  })

  const [activeVersion, setActiveVersion] = useState<string | null>(null)
  const sectionRefs = useRef(new Map<string, HTMLElement>())

  // 滚动跟随：内容区滚动时高亮左侧对应版本（顶部 20% 视口带内的章节视为当前）
  useEffect(() => {
    if (!open || !data || data.length === 0) return
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setActiveVersion(entry.target.getAttribute('data-version'))
          }
        }
      },
      { rootMargin: '0px 0px -80% 0px' },
    )
    sectionRefs.current.forEach((el) => observer.observe(el))
    return () => observer.disconnect()
  }, [open, data])

  const jumpTo = (version: string) => {
    setActiveVersion(version)
    sectionRefs.current.get(version)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  return (
    <Drawer open={open} onClose={onClose} title="更新日志" description="版本发布记录，随当前运行版本内置。点击左侧版本号快速定位。" width="xl">
      {isLoading && <p className="py-10 text-center text-sm text-soft">加载中…</p>}
      {error && <p className="py-10 text-center text-sm text-[var(--danger)]">{(error as Error).message}</p>}
      {data && data.length === 0 && <p className="py-10 text-center text-sm text-soft">暂无更新日志</p>}
      {data && data.length > 0 && (
        <div className="grid gap-6 md:grid-cols-[150px_minmax(0,1fr)]">
          {/* 左侧：版本导航（随抽屉内容区滚动吸顶） */}
          <nav className="hidden self-start md:sticky md:top-0 md:block md:max-h-[calc(100dvh-180px)] md:overflow-y-auto md:pr-1">
            <div className="space-y-0.5">
              {data.map((note, index) => {
                const active = activeVersion ? activeVersion === note.version : index === 0
                return (
                  <button
                    key={note.version}
                    type="button"
                    onClick={() => jumpTo(note.version)}
                    className={cn(
                      'flex w-full items-center justify-between rounded-md border-l-2 px-3 py-1.5 text-left text-sm transition',
                      active
                        ? 'border-[var(--accent)] bg-[var(--accent-soft)] font-semibold text-[var(--accent)]'
                        : 'border-transparent text-soft hover:bg-[var(--panel-muted)] hover:text-[var(--text)]',
                    )}
                  >
                    <span>{note.version}</span>
                    {index === 0 && <span className="text-[10px] text-[var(--success)]">最新</span>}
                  </button>
                )
              })}
            </div>
          </nav>

          {/* 右侧：日志内容 */}
          <div className="min-w-0 space-y-8">
            {data.map((note, index) => (
              <section
                key={note.version}
                data-version={note.version}
                ref={(el) => {
                  if (el) sectionRefs.current.set(note.version, el)
                  else sectionRefs.current.delete(note.version)
                }}
                className="scroll-mt-2"
              >
                <div className="mb-3 flex items-center gap-2">
                  <span className="text-base font-semibold tracking-[-0.02em]">{note.version}</span>
                  {index === 0 && <Badge label="当前最新" variant="green" />}
                </div>
                <MarkdownLite content={note.content} />
                {index < data.length - 1 && <div className="mt-8 h-px bg-[var(--border)]" />}
              </section>
            ))}
          </div>
        </div>
      )}
    </Drawer>
  )
}

// MarkdownLite 极简 Markdown 渲染：只覆盖 release-notes 实际用到的语法
// （## / ### 标题、- 列表、**加粗**、`行内代码`、段落），不引入第三方渲染库。
// 文件首行的 "# xray-pilot vX.Y.Z" 与版本标题重复，跳过不渲染。
function MarkdownLite({ content }: { content: string }) {
  const lines = content.split(/\r?\n/)
  const blocks: ReactNode[] = []
  let listItems: string[] = []

  const flushList = () => {
    if (listItems.length === 0) return
    blocks.push(
      <ul key={blocks.length} className="ml-4 list-disc space-y-1 text-sm leading-6 text-soft">
        {listItems.map((item, i) => (
          <li key={i}>{renderInline(item)}</li>
        ))}
      </ul>,
    )
    listItems = []
  }

  for (const raw of lines) {
    const line = raw.trimEnd()
    const listMatch = /^\s*[-*]\s+(.*)$/.exec(line)
    if (listMatch) {
      listItems.push(listMatch[1])
      continue
    }
    flushList()
    if (line.trim() === '') continue
    if (line.startsWith('# ')) continue
    if (line.startsWith('### ')) {
      blocks.push(
        <h5 key={blocks.length} className="mt-4 text-sm font-semibold text-[var(--text)]">
          {renderInline(line.slice(4))}
        </h5>,
      )
      continue
    }
    if (line.startsWith('## ')) {
      blocks.push(
        <h4 key={blocks.length} className="mt-5 text-sm font-semibold uppercase tracking-[0.08em] text-[var(--accent)]">
          {renderInline(line.slice(3))}
        </h4>,
      )
      continue
    }
    blocks.push(
      <p key={blocks.length} className="text-sm leading-6 text-soft">
        {renderInline(line)}
      </p>,
    )
  }
  flushList()

  return <div className="space-y-2.5">{blocks}</div>
}

// renderInline 处理行内 **加粗** 与 `代码`；其余按纯文本输出
function renderInline(text: string): ReactNode {
  const parts = text.split(/(\*\*[^*]+\*\*|`[^`]+`)/g)
  return parts.map((part, i) => {
    if (part.startsWith('**') && part.endsWith('**')) {
      return (
        <strong key={i} className="font-semibold text-[var(--text)]">
          {part.slice(2, -2)}
        </strong>
      )
    }
    if (part.startsWith('`') && part.endsWith('`')) {
      return (
        <code key={i} className="rounded bg-[var(--panel-muted)] px-1.5 py-0.5 font-mono text-[12px] text-[var(--text)]">
          {part.slice(1, -1)}
        </code>
      )
    }
    return <Fragment key={i}>{part}</Fragment>
  })
}
