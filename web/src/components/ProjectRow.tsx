import { Link } from '@tanstack/react-router'
import { CaretRightIcon, FolderIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { DeleteProjectDialog } from './DeleteProjectDialog'
import type { ProjectResource } from '../types/projectDetail'

// Shared column grid between the sticky header (routes/projects/index.tsx)
// and every row below, mirroring NODE_LIST_GRID's own shape: unlike
// AppRow/DatabaseRow the row itself is not a whole-row Link, since the
// trailing column holds both a "View" link and DeleteProjectDialog's
// own interactive Dialog trigger as sibling controls (nesting a Dialog
// trigger inside a whole-row Link would nest two interactive elements,
// the same reasoning NodeRow's own comment gives).
export const PROJECT_LIST_GRID =
  'grid grid-cols-[2rem_minmax(0,1.5fr)_minmax(0,1fr)_auto] items-center gap-3'

function formatProjectDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

export function ProjectRow({ project }: { project: ProjectResource }) {
  return (
    <div
      className={`${PROJECT_LIST_GRID} h-full w-full border-b border-border px-4 py-3`}
    >
      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <FolderIcon className="size-4" aria-hidden="true" />
      </span>

      <span className="min-w-0 truncate text-sm font-medium text-foreground">
        {project.name}
      </span>

      <span className="min-w-0 truncate text-xs text-muted-foreground">
        {formatProjectDate(project.created_at)}
      </span>

      <span className="flex shrink-0 items-center gap-2 justify-self-end">
        <Link to="/projects/$id" params={{ id: project.id }}>
          <Button type="button" variant="outline" size="sm">
            View
            <CaretRightIcon className="size-3.5" aria-hidden="true" />
          </Button>
        </Link>
        <DeleteProjectDialog id={project.id} name={project.name} />
      </span>
    </div>
  )
}

// Backs the list route's pendingComponent, matching PROJECT_LIST_GRID
// exactly so the skeleton doesn't jump when real rows swap in, same
// reasoning NodeRow's own RowSkeleton comment gives.
export function RowSkeleton() {
  return (
    <div
      className={`${PROJECT_LIST_GRID} border-b border-border px-4 py-3`}
      aria-hidden="true"
    >
      <div className="size-8 animate-pulse rounded-md bg-muted" />
      <div className="h-4 w-32 animate-pulse rounded bg-muted" />
      <div className="h-4 w-24 animate-pulse rounded bg-muted" />
      <div className="h-6 w-28 animate-pulse justify-self-end rounded bg-muted" />
    </div>
  )
}
