import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { LayersGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { useIsRoot } from '../../hooks/useIsRoot'
import { stacksQueryOptions } from '../../queries/stacks'
import { StackDetail, type StackSelection } from './StackDetail'
import { StacksList } from './StacksList'
import type { StacksSearch } from '../../lib/stacksSearch'

function toSelection(search: StacksSearch): StackSelection {
  const n = Number(search.against)
  return {
    rev: search.rev,
    diff: search.diff === true,
    against: Number.isInteger(n) && n > 0 ? n : 'live',
  }
}

export function StacksPage({
  search,
  onSearchChange,
}: Readonly<{
  search: StacksSearch
  onSearchChange: (next: StacksSearch) => void
}>) {
  const { t } = useTranslation('stacks')
  const isRoot = useIsRoot()
  const { data: stacks, isPending, isError } = useQuery(stacksQueryOptions())
  const selection = toSelection(search)

  return (
    <PageTemplate
      icon={<LayersGlassIcon size="1.4em" />}
      title={t('title')}
      description={t('description')}
    >
      {search.stack ? (
        <StackDetail
          name={search.stack}
          isRoot={isRoot}
          selection={selection}
          onSelect={(next) => {
            const merged = { ...selection, ...next }
            onSearchChange({
              stack: search.stack,
              rev: merged.rev,
              diff: merged.diff ? true : undefined,
              against:
                merged.rev !== undefined && merged.against !== 'live'
                  ? String(merged.against)
                  : undefined,
            })
          }}
          onBack={() => {
            onSearchChange({})
          }}
        />
      ) : isPending ? (
        <TableSkeleton columnCount={5} />
      ) : isError ? (
        <p className="text-sm text-destructive">{t('loadFailed')}</p>
      ) : stacks.length === 0 ? (
        <EmptyState
          className="gap-2 border-0 bg-transparent py-4"
          icon={<LayersGlassIcon className="size-6" />}
          title={t('empty.title')}
          description={t('empty.description')}
        />
      ) : (
        <StacksList
          stacks={stacks}
          onOpen={(name) => {
            onSearchChange({ stack: name })
          }}
        />
      )}
    </PageTemplate>
  )
}
