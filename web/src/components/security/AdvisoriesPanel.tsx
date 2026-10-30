import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ExternalLinkGlassIcon,
  RefreshCwGlassIcon,
  ShieldAlertGlassIcon,
  ShieldGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { HelpLink } from '@/components/HelpLink'
import { formatRelativeTime } from '../../lib/relativeTime'
import {
  advisoriesQueryOptions,
  useAdvisoryRefresh,
  type AdvisoryMatch,
} from '../../queries/securityCenter'

const SEVERITY_ORDER = ['critical', 'high', 'medium', 'low', 'other'] as const
type Bucket = (typeof SEVERITY_ORDER)[number]

const SEVERITY_VARIANT = {
  critical: 'destructive',
  high: 'warning',
  medium: 'info',
  low: 'muted',
  other: 'muted',
} as const

function bucketOf(severity: string): Bucket {
  return SEVERITY_ORDER.find((b) => b === severity) ?? 'other'
}

function shortDigest(d: string): string {
  return d.replace(/^sha256:/, 'sha256:').slice(0, 19)
}

function safeUrl(u: string | undefined): string | null {
  if (!u) return null
  return /^https?:\/\//i.test(u) ? u : null
}

type Outcome = { kind: 'done' | 'pending'; app: string }

export function AdvisoriesPanel() {
  const { t } = useTranslation('advisories')
  const query = useQuery(advisoriesQueryOptions())
  const refresh = useAdvisoryRefresh()
  const [target, setTarget] = useState<AdvisoryMatch | null>(null)
  const [outcome, setOutcome] = useState<Outcome | null>(null)

  if (query.isPending) return <ListSkeleton rows={4} />
  if (query.error || !query.data) {
    return (
      <div className="space-y-2 text-sm" role="alert">
        <p className="text-destructive">{t('loadError')}</p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => void query.refetch()}
        >
          {t('retry')}
        </Button>
      </div>
    )
  }
  const data = query.data
  if (!data.enabled) {
    return (
      <EmptyState
        icon={<ShieldGlassIcon className="size-6" aria-hidden />}
        title={t('offTitle')}
        description={t('offBody')}
        helpPath="/advisory-feed"
        helpLabel={t('offDocs')}
      />
    )
  }

  const signature = data.signature ?? 'unsigned'
  const fetchedAt = data.loaded_at ?? data.generated_at
  const groups = SEVERITY_ORDER.map((b) => ({
    bucket: b,
    rows: data.matches.filter((m) => bucketOf(m.severity) === b),
  })).filter((g) => g.rows.length > 0)

  const closeDialog = () => {
    setTarget(null)
    refresh.reset()
  }
  const confirm = () => {
    if (!target) return
    const app = target.app
    refresh.mutate(app, {
      onSuccess: (res) => {
        setOutcome({ kind: res.pending_approval ? 'pending' : 'done', app })
        setTarget(null)
      },
    })
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <h2 className="text-base font-semibold">{t('title')}</h2>
        <Badge variant={signature === 'verified' ? 'success' : 'warning'}>
          {t('feedLabel')}: {t(`${signature}`)}
        </Badge>
        <span className="text-sm text-muted-foreground">
          {t('count', { count: data.entries.length })}
          {' · '}
          {fetchedAt
            ? t('fetched', { when: formatRelativeTime(fetchedAt) })
            : t('notFetched')}
        </span>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="ml-auto"
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          <RefreshCwGlassIcon className="size-4" aria-hidden />
          {t('refreshFeed')}
        </Button>
        <HelpLink path="/advisory-feed" label={t('offDocs')} variant="inline" />
      </div>
      {data.error ? (
        <p className="text-sm text-destructive" role="alert">
          {t('feedError', { error: data.error })}
        </p>
      ) : null}
      {signature !== 'verified' && data.matches.length > 0 ? (
        <p className="text-sm text-muted-foreground">{t('unverifiedNote')}</p>
      ) : null}
      {outcome ? (
        <div
          role="status"
          className="flex flex-wrap items-center gap-3 rounded-lg border border-tone-info-border bg-tone-info-soft px-3 py-2 text-sm"
        >
          <span>
            {t(outcome.kind === 'pending' ? 'pending' : 'done', {
              app: outcome.app,
            })}
          </span>
          {outcome.kind === 'pending' ? (
            <Link to="/approvals" className="text-primary underline">
              {t('reviewApprovals')}
            </Link>
          ) : null}
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="ml-auto"
            onClick={() => setOutcome(null)}
          >
            {t('dismiss')}
          </Button>
        </div>
      ) : null}
      {groups.length === 0 ? (
        <EmptyState
          compact
          icon={<ShieldGlassIcon className="size-5" aria-hidden />}
          title={t('emptyTitle')}
          description={t('emptyBody')}
        />
      ) : (
        groups.map((g) => (
          <section key={g.bucket} className="space-y-1">
            <h3 className="flex items-center gap-2 text-sm font-medium">
              <ShieldAlertGlassIcon className="size-4" aria-hidden />
              <Badge variant={SEVERITY_VARIANT[g.bucket]}>
                {t(`severity.${g.bucket}`)}
              </Badge>
              <span className="text-xs text-muted-foreground">
                {t('groupCount', { count: g.rows.length })}
              </span>
            </h3>
            <div className="overflow-x-auto rounded-lg border">
              <table className="w-full text-left text-sm">
                <thead className="bg-muted/40 text-xs text-muted-foreground">
                  <tr>
                    <th scope="col" className="px-3 py-2 font-medium">
                      {t('columns.app')}
                    </th>
                    <th scope="col" className="px-3 py-2 font-medium">
                      {t('columns.image')}
                    </th>
                    <th scope="col" className="px-3 py-2 font-medium">
                      {t('columns.advisory')}
                    </th>
                    <th scope="col" className="px-3 py-2 font-medium">
                      {t('columns.fixedIn')}
                    </th>
                    <th
                      scope="col"
                      className="px-3 py-2 text-right font-medium"
                    >
                      {t('columns.action')}
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {g.rows.map((m) => {
                    const url = safeUrl(m.reference)
                    return (
                      <tr key={`${m.app}:${m.advisory_id}`}>
                        <td className="px-3 py-2 font-medium">{m.app}</td>
                        <td className="px-3 py-2">
                          <span className="break-all font-mono text-xs">
                            {m.image}
                          </span>
                          {m.digest ? (
                            <span
                              className="ml-2 font-mono text-xs text-muted-foreground"
                              title={t('digestTitle', {
                                digest: m.digest,
                              })}
                            >
                              {shortDigest(m.digest)}
                            </span>
                          ) : null}
                        </td>
                        <td className="px-3 py-2">
                          {url ? (
                            <a
                              href={url}
                              target="_blank"
                              rel="noreferrer"
                              className="inline-flex items-center gap-1 text-primary underline"
                              aria-label={t('reference', {
                                id: m.advisory_id,
                              })}
                            >
                              {m.advisory_id}
                              <ExternalLinkGlassIcon
                                className="size-3.5"
                                aria-hidden
                              />
                            </a>
                          ) : (
                            m.advisory_id
                          )}
                        </td>
                        <td className="px-3 py-2">
                          {m.fixed_in ?? (
                            <span className="text-muted-foreground">
                              {t('noFix')}
                            </span>
                          )}
                        </td>
                        <td className="px-3 py-2 text-right">
                          <Button
                            type="button"
                            size="sm"
                            variant="outline"
                            writeAction
                            aria-label={t('refreshLabel', {
                              app: m.app,
                            })}
                            onClick={() => {
                              setOutcome(null)
                              setTarget(m)
                            }}
                          >
                            <RefreshCwGlassIcon
                              className="size-4"
                              aria-hidden
                            />
                            {t('refresh')}
                          </Button>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </section>
        ))
      )}
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open) closeDialog()
        }}
        title={t('confirmTitle', { app: target?.app ?? '' })}
        description={t('confirmDescription', {
          app: target?.app ?? '',
          image: target?.image ?? '',
          id: target?.advisory_id ?? '',
        })}
        consequences={[
          t('confirmWhat'),
          t('confirmDeploy'),
          t('confirmApproval'),
          t('confirmNoFix'),
        ]}
        confirmLabel={t('confirmAction')}
        pendingLabel={t('confirmPending')}
        pending={refresh.isPending}
        error={refresh.error?.message ?? null}
        onConfirm={confirm}
      />
    </div>
  )
}
