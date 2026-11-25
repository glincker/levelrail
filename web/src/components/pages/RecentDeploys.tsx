import { lazy, Suspense, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
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
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { useDeployProgress } from '../../hooks/useDeployProgress'
import { copyLabelKey, deployCopyOf } from '../../lib/deployCopy'
import { formatAge } from '../../lib/format'
import type { DeployAttempt } from '../../types/deployAttempt'
import {
  buildSeconds,
  deployStateKey,
  SHORT_SHA,
  splitDuration,
} from './overviewHelpers'

const DeployRowActions = lazy(() => import('./DeployRowActions'))
const DeployLogDialog = lazy(() => import('./DeployLogDialog'))
const DeployFailTail = lazy(() => import('./DeployFailTail'))

const SHOWN = 5
/** Lines of a failed build shown inline under its row. */
const FAIL_TAIL_LINES = 5
/** Trailing lines the fix-hint matcher reads, wider than the shown tail. */
const FAIL_SCAN_LINES = 80
const COLUMNS = 8

const DOT: Record<string, string> = {
  succeeded: 'bg-success',
  running: 'bg-warning motion-safe:animate-pulse',
  failed: 'bg-destructive',
}

function CopySha({ attempt }: Readonly<{ attempt: DeployAttempt }>) {
  const { t } = useTranslation('pagesSites')
  const { copied, copy } = useCopyToClipboard()
  const full = attempt.commit_sha ?? attempt.id
  return (
    <span className="inline-flex items-center gap-0.5">
      <code className="font-mono text-xs">{full.slice(0, SHORT_SHA)}</code>
      <Button
        size="icon-sm"
        variant="ghost"
        aria-label={copied ? t('recent.copied') : t('recent.copySha')}
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
  const { t } = useTranslation('pagesSites')
  const secs = deployCopyOf(attempt)
    ? null
    : buildSeconds(attempt.started_at, attempt.finished_at)
  const dur = secs === null ? null : splitDuration(secs)
  return (
    <span className="inline-flex items-center gap-1.5 text-xs">
      <span
        aria-hidden="true"
        className={`size-2 shrink-0 rounded-full ${DOT[attempt.status] ?? 'bg-muted-foreground/50'}`}
      />
      <span>{t(`overview.state.${deployStateKey(attempt.status)}`)}</span>
      {dur ? (
        <span className="text-muted-foreground tabular-nums">
          {t('overview.duration', dur)}
        </span>
      ) : null}
    </span>
  )
}

/** Latest deploys as a compact table; a row opens the deploy, Promote and the deploy URL sit in its last cell. */
export function RecentDeploys({
  name,
  onViewAll,
}: Readonly<{ name: string; onViewAll: () => void }>) {
  const { t } = useTranslation('pagesSites')
  const navigate = useNavigate()
  const { attempts } = useDeployProgress(name)
  const [logFor, setLogFor] = useState<string | null>(null)
  const rows = attempts.slice(0, SHOWN)
  const open = (deployId: string) =>
    void navigate({
      to: '/apps/$name/deploys/$deployId/logs',
      params: { name, deployId },
    })
  return (
    <section
      aria-labelledby="pages-recent-title"
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex items-center justify-between gap-2">
        <h2 id="pages-recent-title" className="text-sm font-semibold">
          {t('recent.title')}
        </h2>
        <Button size="xs" variant="ghost" onClick={onViewAll}>
          {t('recent.viewAll')}
        </Button>
      </div>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('recent.none')}</p>
      ) : (
        <Table density="compact">
          <TableHeader>
            <TableRow>
              <TableHead>{t('recent.col.commit')}</TableHead>
              <TableHead>{t('recent.col.message')}</TableHead>
              <TableHead>{t('recent.col.state')}</TableHead>
              <TableHead>
                <span className="sr-only">{t('logs.action')}</span>
              </TableHead>
              <TableHead className="hidden sm:table-cell">
                {t('recent.col.branch')}
              </TableHead>
              <TableHead className="hidden xl:table-cell">
                {t('recent.col.author')}
              </TableHead>
              <TableHead>{t('recent.col.when')}</TableHead>
              <TableHead>
                <span className="sr-only">{t('recent.col.actions')}</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((a) => (
              <DeployRow
                key={a.id}
                name={name}
                attempt={a}
                onOpen={() => open(a.id)}
                onLog={() => setLogFor(a.id)}
              />
            ))}
          </TableBody>
        </Table>
      )}
      {logFor ? (
        <Suspense fallback={null}>
          <DeployLogDialog
            name={name}
            deployId={logFor}
            open
            onOpenChange={(o) => {
              if (!o) setLogFor(null)
            }}
          />
        </Suspense>
      ) : null}
    </section>
  )
}

function DeployRow({
  name,
  attempt: a,
  onOpen,
  onLog,
}: Readonly<{
  name: string
  attempt: DeployAttempt
  onOpen: () => void
  onLog: () => void
}>) {
  const { t } = useTranslation('pagesSites')
  const { t: td } = useTranslation('deploys')
  const copy = deployCopyOf(a)
  const copyLabel = copy
    ? td(copyLabelKey(copy), { id: copy.target })
    : undefined
  const message = copyLabel ?? a.commit_message?.split('\n')[0]
  return (
    <>
      <TableRow
        data-testid="deploy-row"
        className="cursor-pointer"
        onClick={(e) => {
          // Buttons and links inside the row keep their own action.
          if ((e.target as HTMLElement).closest('a,button')) return
          onOpen()
        }}
      >
        <TableCell>
          <CopySha attempt={a} />
        </TableCell>
        <TableCell>
          <span className="block max-w-56 truncate text-xs" title={message}>
            {a.live ? (
              <Badge className="mr-1.5">{t('recent.live')}</Badge>
            ) : null}
            {message ?? (
              <span className="text-muted-foreground">
                {a.source ?? t('overview.strip.noMessage')}
              </span>
            )}
          </span>
        </TableCell>
        <TableCell>
          <StateCell attempt={a} />
        </TableCell>
        <TableCell>
          <Button size="xs" variant="ghost" onClick={onLog}>
            {t('logs.action')}
          </Button>
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
          {formatAge(a.finished_at ?? a.started_at)}
        </TableCell>
        <TableCell>
          {a.status === 'succeeded' ? (
            <Suspense fallback={null}>
              <DeployRowActions name={name} deployId={a.id} />
            </Suspense>
          ) : null}
        </TableCell>
      </TableRow>
      {a.status === 'failed' ? (
        <TableRow className="hover:bg-transparent">
          <TableCell
            colSpan={COLUMNS}
            className="h-auto py-0 whitespace-normal"
          >
            <Suspense fallback={null}>
              <DeployFailTail
                name={name}
                deployId={a.id}
                lines={FAIL_TAIL_LINES}
                scan={FAIL_SCAN_LINES}
              />
            </Suspense>
          </TableCell>
        </TableRow>
      ) : null}
    </>
  )
}
