import { createFileRoute } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useRef } from 'react'
import { FolderGlassIcon } from '@thesvg/ui-react'
import { projectListQueryOptions, useProjects } from '../../queries/projects'
import {
  PROJECT_LIST_GRID,
  ProjectRow,
  RowSkeleton,
} from '../../components/ProjectRow'
import { CreateProjectDialog } from '../../components/CreateProjectDialog'
import { useTranslation } from 'react-i18next'
import { PageBanner } from '../../components/PageBanner'
import { LazyProjectsDiagram } from '../../components/banners/LazyDiagrams'
import { PageTemplate } from '../../components/shell/PageTemplate'

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

  const { t } = useTranslation('common')
  const isEmpty = projects.length === 0
  const size = isEmpty ? 'hero' : 'compact'

  return (
    <PageTemplate
      title={t('banners.projects.title')}
      banner={
        <PageBanner
          icon={<FolderGlassIcon size="1.4em" />}
          titleKey="common:banners.projects.title"
          descriptionKey={
            isEmpty
              ? 'common:banners.projects.description'
              : 'common:banners.projects.compactDescription'
          }
          descriptionText={
            isEmpty
              ? undefined
              : t('banners.projects.count', { count: projects.length })
          }
          action={<CreateProjectDialog />}
          illustration={<LazyProjectsDiagram size={size} />}
          size={size}
        />
      }
    >
      {isEmpty ? null : (
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
    </PageTemplate>
  )
}

function ProjectListPending() {
  return (
    <PageTemplate title="Projects">
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <ListHeader />
        {Array.from({ length: 3 }, (_, i) => (
          <RowSkeleton key={i} />
        ))}
      </div>
    </PageTemplate>
  )
}
