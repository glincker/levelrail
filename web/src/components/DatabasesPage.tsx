import { lazy, Suspense, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { DatabaseIcon, WarningCircleIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import type { DatabaseListEntry } from '../types/databaseDetail'
import { ConnectDatabaseDialog } from './ConnectDatabaseDialog'
import { CreateDatabaseDialog } from './CreateDatabaseDialog'
import { DatabasesTable } from './DatabasesTable'
import { PageBanner, type PageBannerSize } from './PageBanner'

const DatabasesDiagram = lazy(() =>
  import('./banners/DatabasesDiagram').then((m) => ({
    default: m.DatabasesDiagram,
  })),
)

const DESCRIPTION_KEY = {
  banner: 'databases:banner.description',
  list: 'databases:list.description',
} as const

export function DatabasesShell({
  size,
  empty,
  children,
}: Readonly<{
  size: PageBannerSize
  empty: boolean
  children: ReactNode
}>) {
  return (
    <div className="mx-auto flex w-full min-w-0 max-w-(--glinr-content-max) flex-col gap-(--glinr-page-gap)">
      <PageBanner
        icon={<DatabaseIcon />}
        titleKey="databases:banner.title"
        descriptionKey={DESCRIPTION_KEY[empty ? 'banner' : 'list']}
        action={
          <>
            <ConnectDatabaseDialog />
            <CreateDatabaseDialog />
          </>
        }
        illustration={
          <Suspense fallback={null}>
            <DatabasesDiagram size={size} />
          </Suspense>
        }
        size={size}
      />
      {children}
    </div>
  )
}

export function DatabasesPage({
  databases,
}: Readonly<{ databases: readonly DatabaseListEntry[] }>) {
  const { t } = useTranslation('databases')
  const isEmpty = databases.length === 0
  return (
    <DatabasesShell size={isEmpty ? 'hero' : 'compact'} empty={isEmpty}>
      {isEmpty ? (
        <p className="max-w-prose text-sm text-muted-foreground">
          {t('banner.whatFor')}
        </p>
      ) : (
        <DatabasesTable databases={databases} />
      )}
    </DatabasesShell>
  )
}

export function DatabasesPending() {
  const { t } = useTranslation('databases')
  return (
    <DatabasesShell size="compact" empty={false}>
      <div
        role="status"
        aria-label={t('list.loading')}
        className="space-y-2 rounded-xl border border-border bg-card p-4"
      >
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-12 w-full" />
        ))}
      </div>
    </DatabasesShell>
  )
}

export function DatabasesError({ message }: Readonly<{ message: string }>) {
  const { t } = useTranslation('databases')
  return (
    <DatabasesShell size="compact" empty={false}>
      <EmptyState
        icon={<WarningCircleIcon />}
        title={t('list.errorTitle')}
        description={`${t('list.errorBody')} ${message}`}
        action={
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              globalThis.location.reload()
            }}
          >
            {t('list.retry')}
          </Button>
        }
      />
    </DatabasesShell>
  )
}
