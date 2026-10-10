import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowCounterClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { StatusPill, type Tone } from '@/components/kit'
import { Badge } from '../ui/badge'
import { Button } from '../ui/button'
import { Tabs, TabsList, TabsTrigger } from '../ui/tabs'
import {
  releaseHistoryQueryOptions,
  type HistoryView,
  type ReleaseHistoryItem,
  type SchemaVerdict,
} from '../../queries/releases'
import { RollbackPlanDialog } from './RollbackPlanDialog'

const VERDICT_TONE: Record<SchemaVerdict, Tone> = {
  binary_only: 'success',
  forward: 'info',
  restore_required: 'warning',
  unknown: 'neutral',
}

const VIEWS: HistoryView[] = ['stable', 'beta', 'all']

function megabytes(bytes: number): string {
  return (bytes / (1024 * 1024)).toFixed(1)
}

function ReleaseRow({
  release,
  onPreview,
}: {
  release: ReleaseHistoryItem
  onPreview: (version: string) => void
}) {
  const { t } = useTranslation('updates')
  const published = release.published_at
    ? new Date(release.published_at).toLocaleDateString()
    : null
  return (
    <li className="flex flex-wrap items-center justify-between gap-3 py-3">
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-mono text-sm text-foreground">
            {release.version}
          </span>
          <Badge variant="outline">
            {t(`history.channel.${release.channel}`)}
          </Badge>
          {release.running ? <Badge>{t('history.running')}</Badge> : null}
          {!release.signed && release.asset_available && !release.retained ? (
            <Badge variant="outline">{t('history.unsigned')}</Badge>
          ) : null}
        </div>
        <p className="text-xs text-muted-foreground">
          {[
            published,
            release.schema_version !== null
              ? t('history.schema', { version: release.schema_version })
              : t('history.schemaUnknown'),
            release.retained
              ? `${t('history.retained')}, ${t('history.retainedHelp')}`
              : release.asset_available
                ? release.asset_size > 0
                  ? `${t('history.notRetained')} (${t('history.binarySize', { size: megabytes(release.asset_size) })})`
                  : t('history.notRetained')
                : t('history.noBinary'),
          ]
            .filter((s): s is string => Boolean(s))
            .join(' | ')}
        </p>
      </div>
      <div className="flex items-center gap-3">
        <StatusPill
          tone={VERDICT_TONE[release.verdict]}
          size="sm"
          label={t(`history.verdict.${release.verdict}`)}
        />
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={release.running || !release.asset_available}
          aria-label={
            release.running
              ? t('history.previewRunning')
              : t('history.previewFor', { version: release.version })
          }
          onClick={() => {
            onPreview(release.version)
          }}
        >
          <ArrowCounterClockwiseIcon className="size-4" />
          {t('history.preview')}
        </Button>
      </div>
    </li>
  )
}

export function ReleaseHistory() {
  const { t } = useTranslation('updates')
  const [view, setView] = useState<HistoryView | null>(null)
  const [previewing, setPreviewing] = useState<string | null>(null)
  const { data, isPending, isError, error, refetch } = useQuery(
    releaseHistoryQueryOptions(view),
  )
  const activeView: HistoryView = view ?? data?.view ?? 'stable'

  return (
    <div className="space-y-4">
      <Tabs
        value={activeView}
        onValueChange={(v) => {
          const next = String(v)
          if (next === 'stable' || next === 'beta' || next === 'all') {
            setView(next)
          }
        }}
      >
        <TabsList aria-label={t('history.viewLabel')}>
          {VIEWS.map((v) => (
            <TabsTrigger key={v} value={v}>
              {t(`history.view.${v}`)}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      {isPending ? (
        <p className="text-sm text-muted-foreground">{t('history.loading')}</p>
      ) : isError ? (
        <div className="space-y-2">
          <p role="alert" className="text-sm text-destructive">
            {t('history.loadError', { message: error.message })}
          </p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              void refetch()
            }}
          >
            {t('history.retry')}
          </Button>
        </div>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">
            {data.current_schema_version !== null
              ? t('history.currentSchema', {
                  version: data.current_schema_version,
                })
              : t('history.currentSchemaUnknown')}
          </p>
          {!data.github_reachable ? (
            <p role="status" className="text-sm text-muted-foreground">
              {t('history.githubUnreachable')}
            </p>
          ) : null}
          {data.releases.length + data.retained_only.length === 0 ? (
            <div className="rounded-md border border-dashed px-4 py-6 text-center">
              <p className="text-sm font-medium text-foreground">
                {t('history.empty')}
              </p>
              <p className="text-xs text-muted-foreground">
                {t('history.emptyHint')}
              </p>
            </div>
          ) : (
            <ul className="divide-y">
              {[...data.releases, ...data.retained_only].map((r) => (
                <ReleaseRow
                  key={r.version}
                  release={r}
                  onPreview={setPreviewing}
                />
              ))}
            </ul>
          )}
        </>
      )}

      <RollbackPlanDialog
        version={previewing}
        onClose={() => {
          setPreviewing(null)
        }}
      />
    </div>
  )
}
