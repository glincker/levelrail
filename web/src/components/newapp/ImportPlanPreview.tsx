import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowClockwiseIcon,
  ListMagnifyingGlassIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { repoPlanOptions } from '../../queries/importRepoPlan'
import type { GitRepoSourceValue } from '../GitRepoSourcePicker'
import { ImportPlanEditor } from './ImportPlanEditor'

/**
 * Plan panel for a monorepo import: fetched only after the operator asks,
 * because it reads every package from the git host. Creating happens in the
 * editor below, after a dry run.
 */
export function ImportPlanPreview({ source }: { source: GitRepoSourceValue }) {
  const { t } = useTranslation('newApp')
  const [requested, setRequested] = useState(false)
  const query = useQuery(
    repoPlanOptions(
      {
        repo_url: source.repoUrl,
        branch: source.branch || undefined,
        token: source.token,
      },
      requested,
    ),
  )

  if (!requested) {
    return (
      <section className="space-y-2" aria-labelledby="f-plan">
        <h3 id="f-plan" className="text-sm font-semibold">
          {t('plan.title')}
        </h3>
        <p className="text-xs text-muted-foreground">{t('plan.hint')}</p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            setRequested(true)
          }}
        >
          <ListMagnifyingGlassIcon className="size-4" aria-hidden />
          {t('plan.review')}
        </Button>
      </section>
    )
  }

  const rows = query.data?.packages ?? []
  const planWarnings = query.data?.warnings ?? []

  return (
    <section className="space-y-3" aria-labelledby="f-plan">
      <h3 id="f-plan" className="text-sm font-semibold">
        {t('plan.title')}
      </h3>
      {query.isPending ? (
        <p className="text-xs text-muted-foreground" role="status">
          {t('plan.loading')}
        </p>
      ) : null}
      {query.isError ? (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-wrap items-center gap-3">
            <span>{t('plan.error', { message: query.error.message })}</span>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                void query.refetch()
              }}
            >
              <ArrowClockwiseIcon className="size-4" aria-hidden />
              {t('plan.retry')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {query.data ? (
        <>
          {planWarnings.map((w) => (
            <Alert key={w.code}>
              <AlertDescription>
                {t(`plan.warnings.${w.code}`, {
                  defaultValue: w.message,
                  found: query.data.packages_found,
                  cap: query.data.package_cap,
                })}
              </AlertDescription>
            </Alert>
          ))}
          {rows.length === 0 ? (
            <p className="text-xs text-muted-foreground">{t('plan.empty')}</p>
          ) : (
            <>
              <p className="text-xs text-muted-foreground">
                {t('plan.summary', {
                  count: query.data.totals.packages,
                  flagged: query.data.totals.flagged,
                })}
              </p>
              <ImportPlanEditor plan={query.data} source={source} />
            </>
          )}
        </>
      ) : null}
    </section>
  )
}
