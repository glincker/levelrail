import * as React from 'react'
import { ChevronRightGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'

// One row system for every sidebar link: same height, icon column and states as group headers.
export const NAV_ROW_CLASS =
  'h-9 cursor-pointer gap-2.5 rounded-md px-2.5 text-sm text-sidebar-foreground/85 outline-hidden transition-colors duration-150 ease-out motion-reduce:transition-none hover:bg-state-accent/10 hover:text-sidebar-foreground focus-visible:ring-2 focus-visible:ring-sidebar-ring active:bg-state-accent/15 data-active:bg-sidebar-active data-active:font-medium data-active:text-sidebar-active-foreground [&_svg]:text-sidebar-foreground/80 hover:[&_svg]:text-sidebar-foreground data-active:[&_svg]:text-state-accent'

export function NavChevron({
  open,
  className,
}: {
  open: boolean
  className?: string
}) {
  return (
    <ChevronRightGlassIcon
      aria-hidden="true"
      className={cn(
        'size-4 transition-[transform,color] duration-150 ease-out motion-reduce:transition-none',
        open && 'rotate-90',
        className,
      )}
    />
  )
}

// Shared animated expand/collapse container (grid-rows trick, so height
// animates without a measured pixel value) used by CollapsibleSection
// below and by the Projects nav tree (ProjectsNavTree.tsx), so both read
// as the same visual tree instead of two independent implementations.
export function NavCollapsePanel({
  id,
  open,
  children,
}: {
  id: string
  open: boolean
  children: React.ReactNode
}) {
  return (
    <div
      id={id}
      inert={!open}
      className={cn(
        'grid transition-[grid-template-rows,opacity] duration-200 ease-out motion-reduce:transition-none group-data-[collapsible=icon]:hidden',
        open ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0',
      )}
    >
      <div className="min-h-0 overflow-hidden">{children}</div>
    </div>
  )
}

export function CollapsibleSection({
  id,
  open,
  onOpenChange,
  header,
  hint,
  children,
  className,
  active = false,
  chevronClassName,
}: {
  id: string
  open: boolean
  onOpenChange: (open: boolean) => void
  header: React.ReactNode
  // Shown beside the header only while the group is collapsed.
  hint?: React.ReactNode
  children: React.ReactNode
  className?: string
  // Marks a collapsed group that holds the current page.
  active?: boolean
  // Tints the chevron by the group's section hue.
  chevronClassName?: string
}) {
  const panelId = `nav-panel-${id}`
  return (
    <div className={className}>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        title={typeof header === 'string' ? header : undefined}
        data-contains-active={active && !open ? 'true' : undefined}
        onClick={() => onOpenChange(!open)}
        className="group/header flex h-9 w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 text-sm font-medium text-sidebar-foreground/85 outline-hidden transition-colors duration-150 ease-out hover:bg-state-accent/10 hover:text-sidebar-foreground focus-visible:ring-2 focus-visible:ring-sidebar-ring active:bg-state-accent/15 motion-reduce:transition-none data-[contains-active=true]:text-sidebar-active-foreground"
      >
        <span className="grid size-5 shrink-0 place-items-center">
          <NavChevron open={open} className={chevronClassName} />
        </span>
        <span className="min-w-0 flex-1 truncate text-left">{header}</span>
        {active && !open ? (
          <span
            data-testid="group-active-dot"
            aria-hidden="true"
            className="size-1.5 shrink-0 rounded-full bg-state-active-bar"
          />
        ) : null}
        {open ? null : hint}
      </button>
      <NavCollapsePanel id={panelId} open={open}>
        {children}
      </NavCollapsePanel>
    </div>
  )
}
