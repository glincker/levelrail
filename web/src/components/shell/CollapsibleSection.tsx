import * as React from 'react'
import { ChevronRightGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'

// One row system for every sidebar link: same height, icon column and states as group headers.
export const NAV_ROW_CLASS =
  'relative h-(--nav-row-h) cursor-pointer gap-2 rounded-md px-2 before:pointer-events-none before:absolute before:inset-y-2 before:left-0.5 before:w-0.5 before:rounded-full before:bg-state-active-bar before:opacity-0 before:transition-opacity motion-reduce:before:transition-none data-active:before:opacity-100 text-(length:--nav-font) font-normal text-sidebar-foreground/90 outline-hidden transition-colors duration-150 ease-out motion-reduce:transition-none hover:bg-sidebar-accent/50 hover:text-sidebar-foreground focus-visible:ring-2 focus-visible:ring-sidebar-ring data-active:bg-sidebar-accent data-active:font-medium data-active:text-sidebar-foreground [&_svg]:size-4 [&_svg]:text-sidebar-foreground/60 hover:[&_svg]:text-sidebar-foreground/80 data-active:[&_svg]:text-state-accent'

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
        'size-3 transition-[transform,color] duration-150 ease-out motion-reduce:transition-none',
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
    <div className={cn('pt-2', className)}>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        title={typeof header === 'string' ? header : undefined}
        data-contains-active={active && !open ? 'true' : undefined}
        onClick={() => onOpenChange(!open)}
        className="group/header flex h-(--nav-header-h) w-full cursor-pointer items-center gap-2 rounded-md px-2 text-xs font-medium text-sidebar-foreground/70 outline-hidden transition-colors duration-150 ease-out hover:text-sidebar-foreground focus-visible:ring-2 focus-visible:ring-sidebar-ring motion-reduce:transition-none data-[contains-active=true]:text-sidebar-active-foreground"
      >
        <span className="min-w-0 flex-1 truncate text-left">{header}</span>
        {active && !open ? (
          <span
            data-testid="group-active-dot"
            aria-hidden="true"
            className="size-1.5 shrink-0 rounded-full bg-state-active-bar"
          />
        ) : null}
        {open ? null : hint}
        <span className="grid size-3 shrink-0 place-items-center">
          <NavChevron open={open} className={chevronClassName} />
        </span>
      </button>
      <NavCollapsePanel id={panelId} open={open}>
        <div className="pt-1">{children}</div>
      </NavCollapsePanel>
    </div>
  )
}
