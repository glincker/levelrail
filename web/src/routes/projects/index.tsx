import { createFileRoute } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useRef } from 'react'
import { FolderIcon } from '@phosphor-icons/react/dist/ssr'
import { projectListQueryOptions, useProjects } from '../../queries/projects'
import {
  PROJECT_LIST_GRID,
  ProjectRow,
  RowSkeleton,
} from '../../components/ProjectRow'
import { CreateProjectDialog } from '../../components/CreateProjectDialog'

// Typed loader primes the Query cache, the component only reads that
// cache via useProjects() (suspense), mirroring routes/nodes/index.tsx
// exactly. Virtualized unconditionally per the project's own "every
// list over 50 items must be virtualized" rule, the same
// follow-the-rule-even-at-small-scale reasoning NodeListPage's own
// comment gives: a project list realistically stays small for this
// product's 3-50-service target audience, but there's no reason to
// special-case it out of the standing convention.
export const Route = createFileRoute('/projects/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(projectListQueryOptions()),
  component: ProjectListPage,
  pendingComponent: ProjectListPending,
})

function ListHeader() {
  return (
    <div
      className={`${PROJECT_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
    >
      <span aria-hidden="true" />
      <span>Name</span>
      <span>Created</span>
      <span aria-hidden="true" />
    </div>
  )
}

function ProjectListPage() {
  const { data: projects } = useProjects()
  const parentRef = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: projects.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 60,
    overscan: 8,
  })

  return (
    <div>
      <div className="mb-4 flex items-baseline justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold text-foreground">Projects</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Optional labels to group related apps and databases. An app or
            database with no project is just as valid, it still shows up on its
            own list either way.
          </p>
        </div>
        <div className="flex items-baseline gap-3">
          {projects.length > 0 ? (
            <span className="text-sm text-muted-foreground">
              {projects.length} {projects.length === 1 ? 'project' : 'projects'}
            </span>
          ) : null}
          <CreateProjectDialog />
        </div>
      </div>
      {projects.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed border-border bg-card/50 px-4 py-16 text-center">
          <span className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <FolderIcon className="size-5" aria-hidden="true" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">
              No projects yet
            </p>
            <p className="mx-auto max-w-sm text-sm text-muted-foreground">
              Group a web app with its database and cache under one project for
              organization. Nothing about how they run changes.
            </p>
          </div>
          <CreateProjectDialog />
        </div>
      ) : (
        <div
          ref={parentRef}
          className="h-[70vh] overflow-auto rounded-lg border border-border bg-card"
        >
          <ListHeader />
          <div
            style={{
              height: virtualizer.getTotalSize(),
              position: 'relative',
            }}
          >
            {virtualizer.getVirtualItems().map((virtualRow) => {
              const project = projects[virtualRow.index]
              if (!project) {
                return null
              }
              return (
                <div
                  key={virtualRow.key}
                  data-index={virtualRow.index}
                  ref={virtualizer.measureElement}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    transform: `translateY(${virtualRow.start}px)`,
                  }}
                >
                  <ProjectRow project={project} />
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

function ProjectListPending() {
  return (
    <div>
      <div className="mb-4 flex items-baseline justify-between">
        <h1 className="text-lg font-semibold text-foreground">Projects</h1>
      </div>
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <ListHeader />
        {Array.from({ length: 3 }, (_, i) => (
          <RowSkeleton key={i} />
        ))}
      </div>
    </div>
  )
}
