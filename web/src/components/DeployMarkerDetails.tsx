import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import type { DeployAttempt } from '../types/deployAttempt'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from './ui/dialog'
import { buttonVariants } from './ui/button'
import { formatAttemptDuration } from '../lib/observabilityFormat'

function Row({ label, children }: { label: string; children: string }) {
  return (
    <div className="flex justify-between gap-4 py-1.5 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-all text-right font-mono text-xs text-foreground">
        {children}
      </dd>
    </div>
  )
}

// Opens from a deploy marker on any metric chart: what the deploy was, its
// commit and how it ended, with a link to its logs.
export function DeployMarkerDetails({
  appName,
  attempt,
  onClose,
}: {
  appName: string
  attempt: DeployAttempt | null
  onClose: () => void
}) {
  const { t } = useTranslation('observability')
  const duration = attempt ? formatAttemptDuration(attempt) : null
  return (
    <Dialog
      open={attempt !== null}
      onOpenChange={(open) => {
        if (!open) {
          onClose()
        }
      }}
    >
      <DialogContent>
        {attempt ? (
          <>
            <DialogHeader>
              <DialogTitle>
                {t('deployDetails.title', {
                  status: t(`deployStatus.${attempt.status}`),
                })}
              </DialogTitle>
              <DialogDescription>
                {new Date(attempt.started_at).toLocaleString()}
              </DialogDescription>
            </DialogHeader>
            <dl className="divide-y divide-border">
              <Row label={t('deployDetails.image')}>{attempt.image}</Row>
              {attempt.commit_sha ? (
                <Row label={t('deployDetails.commit')}>
                  {attempt.commit_sha.slice(0, 7)}
                </Row>
              ) : null}
              {attempt.source ? (
                <Row label={t('deployDetails.source')}>{attempt.source}</Row>
              ) : null}
              {duration ? (
                <Row label={t('deployDetails.duration')}>{duration}</Row>
              ) : null}
              {attempt.error ? (
                <Row label={t('deployDetails.error')}>{attempt.error}</Row>
              ) : null}
            </dl>
            <Link
              to="/apps/$name/deploys/$deployId/logs"
              params={{ name: appName, deployId: attempt.id }}
              className={buttonVariants({ variant: 'outline', size: 'sm' })}
            >
              {t('deployDetails.openLogs')}
            </Link>
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
