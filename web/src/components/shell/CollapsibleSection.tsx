import * as React from 'react'
import { CaretRightIcon } from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'

export function NavChevron({
  open,
  className,
}: {
  open: boolean
  className?: string
}) {
  return (
    <CaretRightIcon
      aria-hidden="true"
      className={cn(
        'size-3 transition-transform duration-200 ease-out motion-reduce:transition-none',
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
  children,
  className,
}: {
  id: string
  open: boolean
  onOpenChange: (open: boolean) => void
  header: React.ReactNode
  children: React.ReactNode
  className?: string
}) {
  const panelId = `nav-panel-${id}`
  return (
    <div className={className}>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => onOpenChange(!open)}
        className="flex h-7 w-full items-center gap-1 rounded-md px-2 text-xs font-medium text-sidebar-foreground/70 outline-hidden transition-colors hover:text-sidebar-foreground focus-visible:ring-2 focus-visible:ring-sidebar-ring"
      >
        <span className="flex-1 text-left">{header}</span>
        <NavChevron open={open} />
      </button>
      <NavCollapsePanel id={panelId} open={open}>
        {children}
      </NavCollapsePanel>
    </div>
  )
}
