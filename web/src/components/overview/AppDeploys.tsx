import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { RelativeTime } from '@/components/kit'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import type { DeployAttempt } from '../../types/deployAttempt'
import {
  buildSeconds,
  SHORT_SHA,
  splitDuration,
} from '../pages/overviewHelpers'
import { dotClass, stateKey } from './deployState'

const SHOWN = 5

function CopySha({ attempt }: Readonly<{ attempt: DeployAttempt }>) {
  const { t } = useTranslation('appOverview')
  const { copied, copy } = useCopyToClipboard()
  const full = attempt.commit_sha ?? attempt.id
  return (
    <span className="inline-flex items-center gap-0.5">
      <code className="font-mono text-xs">{full.slice(0, SHORT_SHA)}</code>
      <Button
        size="icon-sm"
        variant="ghost"
        aria-label={copied ? t('deploys.copied') : t('deploys.copySha')}
        onClick={() => copy(full)}
      >
        {copied ? (
          <CircleCheckGlassIcon className="size-3.5" aria-hidden="true" />
        ) : (
          <CopyGlassIcon className="size-3.5" aria-hidden="true" />
        )}
      </Button>
    </span>
  )
}

function StateCell({ attempt }: Readonly<{ attempt: DeployAttempt }>) {
  const { t } = useTranslation('appOverview')
  const secs = buildSeconds(attempt.started_at, attempt.finished_at)
  const dur = secs === null ? null : splitDuration(secs)
  return (
    <span className="inline-flex items-center gap-1.5 text-xs">
      <span
        aria-hidden="true"
        className={`size-2 shrink-0 rounded-full ${dotClass(attempt.status)}`}
      />
      <span>{t(`state.${stateKey(attempt.status)}`)}</span>
      {dur ? (
        <span className="text-muted-foreground tabular-nums">
          {t('duration', dur)}
        </span>
      ) : null}
    </span>
  )
}

/** Latest deploys as a compact table; a row opens the deploy logs, Rollback sits in the last cell. */
export function AppDeploys({
  name,
  attempts,
}: Readonly<{ name: string; attempts: DeployAttempt[] }>) {
  const { t } = useTranslation('appOverview')
  const navigate = useNavigate()
  const rows = attempts.slice(0, SHOWN)
  const open = (deployId: string) =>
    void navigate({
      to: '/apps/$name/deploys/$deployId/logs',
      params: { name, deployId },
    })
  return (
    <section
      aria-labelledby="app-deploys-title"
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex items-center justify-between gap-2">
        <h2 id="app-deploys-title" className="text-sm font-semibold">
          {t('deploys.title')}
        </h2>
        <Link
          to="/apps/$name/deploys"
          params={{ name }}
          className={buttonVariants({ variant: 'ghost', size: 'xs' })}
        >
          {t('deploys.viewAll')}
        </Link>
      </div>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('deploys.none')}</p>
      ) : (
        <Table density="compact">
          <TableHeader>
            <TableRow>
              <TableHead>{t('deploys.col.commit')}</TableHead>
              <TableHead>{t('deploys.col.message')}</TableHead>
              <TableHead>{t('deploys.col.state')}</TableHead>
              <TableHead>
                <span className="sr-only">{t('deploys.logs')}</span>
              </TableHead>
              <TableHead className="hidden sm:table-cell">
                {t('deploys.col.branch')}
              </TableHead>
              <TableHead className="hidden xl:table-cell">
                {t('deploys.col.author')}
              </TableHead>
              <TableHead>{t('deploys.col.when')}</TableHead>
              <TableHead>
                <span className="sr-only">{t('deploys.col.actions')}</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((a, i) => (
              <TableRow
                key={a.id}
                data-testid="deploy-row"
                className="cursor-pointer"
                onClick={(e) => {
                  // Buttons and links inside the row keep their own action.
                  if ((e.target as HTMLElement).closest('a,button')) return
                  open(a.id)
                }}
              >
                <TableCell>
                  <CopySha attempt={a} />
                </TableCell>
                <TableCell>
                  <span
                    className="block max-w-56 truncate text-xs"
                    title={a.commit_message?.split('\n')[0]}
                  >
                    {a.live ? (
                      <Badge className="mr-1.5">{t('deploys.live')}</Badge>
                    ) : null}
                    {a.commit_message?.split('\n')[0] ?? (
                      <span className="text-muted-foreground">
                        {a.source ?? t('summary.noMessage')}
                      </span>
                    )}
                  </span>
                </TableCell>
                <TableCell>
                  <StateCell attempt={a} />
                </TableCell>
                <TableCell>
                  <Link
                    to="/apps/$name/deploys/$deployId/logs"
                    params={{ name, deployId: a.id }}
                    className={buttonVariants({ variant: 'ghost', size: 'xs' })}
                  >
                    {t('deploys.logs')}
                  </Link>
                </TableCell>
                <TableCell className="hidden sm:table-cell">
                  {a.branch ? (
                    <Badge variant="outline" className="max-w-32">
                      <span className="truncate">{a.branch}</span>
                    </Badge>
                  ) : null}
                </TableCell>
                <TableCell className="hidden text-xs text-muted-foreground xl:table-cell">
                  {a.author ?? a.source ?? ''}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground tabular-nums">
                  <RelativeTime at={a.finished_at ?? a.started_at} live />
                </TableCell>
                <TableCell>
                  {a.status === 'succeeded' && i > 0 && !a.live ? (
                    <Link
                      to="/apps/$name/deploys"
                      params={{ name }}
                      className={buttonVariants({
                        variant: 'outline',
                        size: 'xs',
                      })}
                    >
                      {t('deploys.rollback')}
                    </Link>
                  ) : null}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  )
}
