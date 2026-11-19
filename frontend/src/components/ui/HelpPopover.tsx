import type { ReactNode } from 'react'
import * as Popover from '@radix-ui/react-popover'
import { CircleHelp } from 'lucide-react'

export function HelpPopover({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Popover.Root>
      <Popover.Trigger
        aria-label={title}
        title={title}
        className="inline-flex size-10 shrink-0 items-center justify-center rounded-md text-soft hover:bg-[var(--panel-muted)] focus-visible:outline-2 focus-visible:outline-[var(--accent)]"
      >
        <CircleHelp aria-hidden="true" className="size-5" />
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          align="end"
          sideOffset={8}
          collisionPadding={12}
          className="z-50 w-[min(22rem,calc(100vw-1.5rem))] space-y-2 rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] p-4 text-sm leading-6 text-soft shadow-lg"
        >
          <Popover.Title className="font-semibold text-[var(--text)]">{title}</Popover.Title>
          <Popover.Description asChild>
            <div className="space-y-2">{children}</div>
          </Popover.Description>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}
