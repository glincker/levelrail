import {
  createFileRoute,
  useRouter,
  type ErrorComponentProps,
} from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useMemo, useRef, useState } from 'react'
import { TriangleAlertGlassIcon, FolderGlassIcon } from '@thesvg/ui-react'
import { projectListQueryOptions, useProjects } from '../../queries/projects'
import {
  PROJECT_LIST_GRID,
  ProjectRow,
  RowSkeleton,
} from '../../components/ProjectRow'
import { ListKeyboardHint } from '../../components/ListKeyboardHint'
import { useListKeyboard } from '../../hooks/useListKeyboard'
import { CreateProjectDialog } from '../../components/CreateProjectDialog'
import { useTranslation } from 'react-i18next'
import { PageBanner } from '../../components/PageBanner'
import { LazyProjectsDiagram } from '../../components/banners/LazyDiagrams'
import { PageTemplate } from '../../components/shell/PageTemplate'
import { EmptyState } from '../../components/ui/empty-state'
import { Button } from '../../components/ui/button'
import { NativeSelect } from '../../components/ui/native-select'
import {
  PROJECT_SORTS,
  isProjectSort,
  loadProjectPins,
  loadProjectSort,
  sortProjects,
  storeProjectPins,
  storeProjectSort,
  type ProjectSort,
} from '../../lib/projectSort'

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
  errorComponent: ProjectListError,
})

function ListHeader() {
  const { t } = useTranslation('common')
  return (
    <div
      className={`${PROJECT_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-[13px] font-medium text-muted-foreground`}
    >
      <span aria-hidden="true" />
      <span>{t('projectsPage.list.name')}</span>
      <span>{t('projectsPage.list.created')}</span>
      <span aria-hidden="true" />
    </div>
  )
}

function ProjectListPage() {
  const { data: unsorted } = useProjects()
  const [sort, setSort] = useState<ProjectSort>(loadProjectSort)
  const [pins, setPins] = useState<string[]>(loadProjectPins)
  const projects = useMemo(
    () => sortProjects(unsorted, sort, pins),
    [unsorted, sort, pins],
  )
  const togglePin = (id: string) => {
    const next = pins.includes(id)
      ? pins.filter((p) => p !== id)
      : [...pins, id]
    setPins(next)
    storeProjectPins(next)
  }
  const parentRef = useRef<HTMLDivElement>(null)
  const {
    setContainer: setListContainer,
    hintOpen,
    closeHint,
  } = useListKeyboard<HTMLDivElement>()

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
      {isEmpty ? (
        <EmptyState
          icon={<FolderGlassIcon className="size-6" />}
          title={t('projectsPage.empty.title')}
          description={t('projectsPage.empty.description')}
          action={<CreateProjectDialog />}
        />
      ) : (
        <>
          <ListKeyboardHint open={hintOpen} onClose={closeHint} />
          <div className="flex items-center justify-end gap-2">
            <label
              htmlFor="projects-sort"
              className="text-xs text-muted-foreground"
            >
              {t('projectsPage.sort.label')}
            </label>
            <NativeSelect
              id="projects-sort"
              value={sort}
              onChange={(e) => {
                const v = e.target.value
                if (!isProjectSort(v)) return
                setSort(v)
                storeProjectSort(v)
              }}
            >
              {PROJECT_SORTS.map((s) => (
                <option key={s} value={s}>
                  {t(`projectsPage.sort.${s}`)}
                </option>
              ))}
            </NativeSelect>
          </div>
          <div
            ref={(el) => {
              parentRef.current = el
              setListContainer(el)
            }}
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
                    data-list-row
                    ref={virtualizer.measureElement}
                    style={{
                      position: 'absolute',
                      top: 0,
                      left: 0,
                      width: '100%',
                      transform: `translateY(${virtualRow.start}px)`,
                    }}
                  >
                    <ProjectRow
                      project={project}
                      pinned={pins.includes(project.id)}
                      onTogglePin={togglePin}
                    />
                  </div>
                )
              })}
            </div>
          </div>
        </>
      )}
    </PageTemplate>
  )
}

function ProjectListError({ error, reset }: ErrorComponentProps) {
  const { t } = useTranslation('common')
  const router = useRouter()
  return (
    <PageTemplate title={t('banners.projects.title')}>
      <EmptyState
        kind="error"
        icon={<TriangleAlertGlassIcon className="size-5" />}
        title={t('projectsPage.error.title')}
        description={t('projectsPage.error.description', {
          error: error instanceof Error ? error.message : '',
        })}
        action={
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              reset()
              void router.invalidate()
            }}
          >
            {t('projectsPage.error.retry')}
          </Button>
        }
      />
    </PageTemplate>
  )
}

function ProjectListPending() {
  const { t } = useTranslation('common')
  return (
    <PageTemplate title={t('banners.projects.title')}>
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <ListHeader />
        {Array.from({ length: 3 }, (_, i) => (
          <RowSkeleton key={i} />
        ))}
      </div>
    </PageTemplate>
  )
}
