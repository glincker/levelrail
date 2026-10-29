import { useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  RefreshCwGlassIcon,
  SearchGlassIcon,
  StoreGlassIcon,
} from '@thesvg/ui-react'
import { BrandTile } from '@/components/BrandLogoBadge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import {
  marketplaceKeys,
  useMarketplace,
  useRefreshMarketplace,
  type MarketplaceEntry,
} from '../../queries/pluginMarketplace'
import { useRemovePlugin } from '../../queries/plugins'
import { MarketplaceInstallDialog } from './MarketplaceInstallDialog'
import { MarketplaceSourcesCard, SourceProblem } from './MarketplaceSourcesCard'

const PAGE_SIZE = 24

function matches(e: MarketplaceEntry, query: string, category: string) {
  if (category && !e.categories.includes(category)) return false
  if (!query) return true
  const hay = `${e.id} ${e.name} ${e.summary} ${e.publisher}`.toLowerCase()
  return hay.includes(query.toLowerCase())
}

function EntryCard({
  entry,
  onInstall,
  onUninstall,
}: Readonly<{
  entry: MarketplaceEntry
  onInstall: (e: MarketplaceEntry) => void
  onUninstall: (e: MarketplaceEntry) => void
}>) {
  const { t } = useTranslation('plugins')
  return (
    <Card>
      <CardContent className="flex h-full flex-col gap-3 pt-4">
        <div className="flex items-start gap-3">
          <BrandTile name={entry.icon} label={entry.name} size="sm" />
          <div className="min-w-0 flex-1">
            <p className="truncate font-medium">{entry.name}</p>
            <p className="truncate text-xs text-muted-foreground">
              {t('publisher', { publisher: entry.publisher })} · {entry.version}
            </p>
          </div>
          {entry.verified ? (
            <Badge variant="success">
              <CircleCheckGlassIcon
                className="text-tone-success"
                size="1.2em"
                aria-hidden="true"
              />
              {t('marketplace.verified')}
            </Badge>
          ) : null}
        </div>
        <p className="text-sm text-muted-foreground">{entry.summary}</p>
        <p className="text-xs text-muted-foreground">
          {t('marketplace.abilitiesSummary', {
            abilities: entry.abilities.join(', '),
          })}
        </p>
        <div className="mt-auto flex flex-wrap items-center gap-2">
          {entry.update_available ? (
            <>
              <Badge variant="warning">
                {t('marketplace.updateAvailable', {
                  version: entry.version,
                })}
              </Badge>
              <Button type="button" size="sm" onClick={() => onInstall(entry)}>
                {t('marketplace.update')}
              </Button>
            </>
          ) : entry.installed ? (
            <Badge variant="muted">
              {t('marketplace.installedState', {
                version: entry.installed_version,
              })}
            </Badge>
          ) : (
            <Button
              type="button"
              size="sm"
              disabled={!entry.compatible}
              onClick={() => onInstall(entry)}
            >
              {t('marketplace.install')}
            </Button>
          )}
          {entry.installed ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => onUninstall(entry)}
            >
              {t('marketplace.uninstall')}
            </Button>
          ) : null}
          {entry.compatible ? null : (
            <span className="text-xs text-destructive">
              {t('marketplace.needsNewer', { version: entry.min_version })}
            </span>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

export function MarketplaceTab() {
  const { t } = useTranslation('plugins')
  const { data, isLoading, error, refetch } = useMarketplace()
  const refresh = useRefreshMarketplace()
  const remove = useRemovePlugin()
  const qc = useQueryClient()
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState('')
  const [shown, setShown] = useState(PAGE_SIZE)
  const [installing, setInstalling] = useState<MarketplaceEntry | null>(null)
  const [uninstalling, setUninstalling] = useState<MarketplaceEntry | null>(
    null,
  )

  const entries = data?.entries
  const categories = useMemo(
    () => [...new Set((entries ?? []).flatMap((e) => e.categories))].sort(),
    [entries],
  )
  const hits = useMemo(
    () => (entries ?? []).filter((e) => matches(e, query, category)),
    [entries, query, category],
  )
  const sources = data?.sources ?? []

  return (
    <div className="space-y-4">
      {sources.map((s) => (
        <SourceProblem key={s.id} source={s} />
      ))}
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-48 flex-1">
          <SearchGlassIcon
            aria-hidden="true"
            className="absolute top-2 left-2.5 size-4.5 text-muted-foreground"
          />
          <Input
            type="search"
            aria-label={t('marketplace.search')}
            placeholder={t('marketplace.search')}
            className="pl-8"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setShown(PAGE_SIZE)
            }}
          />
        </div>
        <Button
          type="button"
          variant="outline"
          disabled={refresh.isPending}
          onClick={() => refresh.mutate('')}
        >
          <RefreshCwGlassIcon size="1.2em" aria-hidden="true" />
          {refresh.isPending
            ? t('marketplace.rechecking')
            : t('marketplace.recheck')}
        </Button>
      </div>
      {categories.length > 0 ? (
        <div
          className="flex flex-wrap gap-2"
          role="group"
          aria-label={t('marketplace.categories')}
        >
          {['', ...categories].map((c) => (
            <button
              key={c || 'all'}
              type="button"
              aria-pressed={category === c}
              onClick={() => {
                setCategory(c)
                setShown(PAGE_SIZE)
              }}
              className={cn(
                'rounded-full border px-3 py-0.5 text-xs',
                category === c
                  ? 'border-primary bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:text-foreground',
              )}
            >
              {c === '' ? t('marketplace.allCategories') : c}
            </button>
          ))}
        </div>
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <AlertTitle>{t('marketplace.loadFailed')}</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>{error.message}</p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void refetch()}
            >
              {t('marketplace.retry')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : isLoading ? (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <Skeleton className="h-36" />
          <Skeleton className="h-36" />
          <Skeleton className="h-36" />
        </div>
      ) : hits.length === 0 ? (
        <EmptyState
          className="gap-2 border-0 bg-transparent py-4"
          icon={<StoreGlassIcon className="size-5.5" />}
          title={t(
            sources.length === 0
              ? 'marketplace.noSources.title'
              : 'marketplace.empty.title',
          )}
          description={t(
            sources.length === 0
              ? 'marketplace.noSources.description'
              : 'marketplace.empty.description',
          )}
        />
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {hits.slice(0, shown).map((e) => (
              <EntryCard
                key={`${e.source_id}/${e.id}`}
                entry={e}
                onInstall={setInstalling}
                onUninstall={setUninstalling}
              />
            ))}
          </div>
          {hits.length > shown ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => setShown((n) => n + PAGE_SIZE)}
            >
              {t('marketplace.showMore', { count: hits.length - shown })}
            </Button>
          ) : null}
        </>
      )}
      <MarketplaceSourcesCard sources={sources} />
      <MarketplaceInstallDialog
        entry={installing}
        onClose={() => setInstalling(null)}
      />
      <ConfirmDialog
        open={uninstalling !== null}
        onOpenChange={(open) => (open ? undefined : setUninstalling(null))}
        title={t('remove.title', { name: uninstalling?.id })}
        description={t('remove.description')}
        confirmLabel={t('remove.confirm')}
        tone="destructive"
        pending={remove.isPending}
        error={remove.error?.message ?? null}
        onConfirm={() => {
          if (!uninstalling) return
          remove.mutate(uninstalling.id, {
            onSuccess: () => {
              toast.add({
                type: 'success',
                title: t('marketplace.uninstalled', {
                  name: uninstalling.name,
                }),
              })
              void qc.invalidateQueries({ queryKey: marketplaceKeys.catalog })
              setUninstalling(null)
            },
          })
        }}
      />
    </div>
  )
}
