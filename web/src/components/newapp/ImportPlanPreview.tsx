import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowClockwiseIcon,
  ListMagnifyingGlassIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  repoPlanOptions,
  type RepoPlanWarning,
} from '../../queries/importRepoPlan'
import type { GitRepoSourceValue } from '../GitRepoSourcePicker'

/**
 * Read-only preview of the apps an import would propose, one per workspace
 * package. Fetched only after the operator asks, because it reads every
 * package from the git host. Creates nothing.
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
  const warnText = (w: RepoPlanWarning) =>
    t(`plan.warnings.${w.code}`, { defaultValue: w.message })

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
              <div className="overflow-x-auto rounded-md border border-border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('plan.columns.name')}</TableHead>
                      <TableHead>{t('plan.columns.directory')}</TableHead>
                      <TableHead>{t('plan.columns.port')}</TableHead>
                      <TableHead>{t('plan.columns.build')}</TableHead>
                      <TableHead>{t('plan.columns.dependsOn')}</TableHead>
                      <TableHead>{t('plan.columns.warnings')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {rows.map((r) => (
                      <TableRow key={r.directory}>
                        <TableCell className="font-medium">{r.name}</TableCell>
                        <TableCell className="font-mono text-xs">
                          {r.directory}
                        </TableCell>
                        <TableCell title={r.port_source}>
                          {r.port ?? t('plan.none')}
                        </TableCell>
                        <TableCell>
                          <Badge variant="muted">
                            {t(`plan.kinds.${r.build_kind}`)}
                          </Badge>
                        </TableCell>
                        <TableCell className="font-mono text-xs">
                          {r.depends_on.length > 0
                            ? r.depends_on.join(', ')
                            : t('plan.none')}
                        </TableCell>
                        <TableCell>
                          {r.warnings.length === 0 ? (
                            t('plan.none')
                          ) : (
                            <ul className="space-y-0.5 text-xs">
                              {r.warnings.map((w) => (
                                <li
                                  key={w.code}
                                  className="flex items-start gap-1"
                                >
                                  <WarningIcon
                                    className="mt-0.5 size-3 shrink-0"
                                    aria-hidden
                                  />
                                  {warnText(w)}
                                </li>
                              ))}
                            </ul>
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </>
          )}
        </>
      ) : null}
    </section>
  )
}
