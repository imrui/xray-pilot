import { useEffect, useRef, useState } from 'react'
import { Server, Users } from 'lucide-react'
import { SearchInput } from '@/components/ui/SearchInput'

export function HeaderSearch({ onNavigate, pathname }: {
  onNavigate: (path: string) => void
  pathname: string
}) {
  const [text, setText] = useState('')
  const [open, setOpen] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const keyword = text.trim()
  const targets = [
    { path: '/nodes', label: '在节点中搜索', Icon: Server },
    { path: '/users', label: '在用户中搜索', Icon: Users },
  ]
  if (pathname === '/users') targets.reverse()

  useEffect(() => {
    const focusSearch = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k' && input.current?.offsetWidth && !document.querySelector('[role="dialog"]')) {
        event.preventDefault()
        input.current.focus()
        input.current.select()
      }
    }
    document.addEventListener('keydown', focusSearch)
    return () => document.removeEventListener('keydown', focusSearch)
  }, [])

  function search(path: string) {
    if (!keyword) return
    setOpen(false)
    onNavigate(`${path}?${new URLSearchParams({ q: keyword })}`)
  }

  return (
    <form
      role="search"
      aria-label="搜索节点或用户"
      className="relative w-64"
      onSubmit={(event) => {
        event.preventDefault()
        search(targets[0].path)
      }}
      onFocus={() => setOpen(true)}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false)
      }}
      onKeyDown={(event) => {
        if (event.key === 'Escape') {
          event.preventDefault()
          input.current?.focus()
          setOpen(false)
        }
      }}
    >
      <SearchInput
        inputRef={input}
        value={text}
        onChange={(value) => {
          setText(value)
          setOpen(true)
        }}
        placeholder="搜索节点、用户..."
        inputClassName="h-9 bg-[var(--panel)]"
      />
      {!text && <kbd className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-faint">{navigator.platform.startsWith('Mac') ? '⌘K' : 'Ctrl K'}</kbd>}
      {open && keyword && (
        <div className="absolute inset-x-0 top-full z-50 mt-2 grid gap-1 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-1.5 shadow-[var(--shadow-panel)]">
          {targets.map(({ path, label, Icon }, index) => (
            <button
              key={path}
              type={index === 0 ? 'submit' : 'button'}
              onClick={index === 0 ? undefined : () => search(path)}
              className="flex items-center gap-2 rounded px-3 py-2 text-left text-sm hover:bg-[var(--panel-muted)] focus-visible:outline-[var(--accent)]"
            >
              <Icon aria-hidden="true" className="size-4 text-soft" />
              {label}
            </button>
          ))}
        </div>
      )}
    </form>
  )
}
