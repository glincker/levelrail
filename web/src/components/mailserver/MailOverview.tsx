import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import type { MailCheckReport } from '@/queries/mailDns'
import { isMailUp, type MailStatus } from '@/queries/mailStatus'
import { useMailBackups } from '../../queries/mailBackup'
import { useMailQueue } from '../../queries/mailMailboxes'
import { MailChecklist } from './MailChecklist'
import { MailNextStep } from './MailNextStep'
import { nextStepOf } from './mailSetup'
import { MailPortsCard } from './MailPortsCard'
import type { MailTab } from './mailTabs'

interface Props {
  status: MailStatus
  report: MailCheckReport | undefined
  domain: string
  onPreview: () => void
  onTab: (tab: MailTab) => void
}

const chipClass =
  'inline-flex items-center gap-1 rounded-full border border-border bg-card px-2.5 py-1 text-xs hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring'

/** Calm landing: stat chips plus one next step, collapsing to a single line once set up. */
export function MailOverview({
  status,
  report,
  domain,
  onPreview,
  onTab,
}: Props) {
  const { t } = useTranslation('mailServer')
  const up = isMailUp(status)
  const [details, setDetails] = useState(false)
  const backups = useMailBackups(up ? (status.app ?? '') : '').data
  const queue = useMailQueue(up).data
  const done = up && nextStepOf(status, report) === null
  const items = report?.items ?? []
  const pass = items.filter((i) => i.status === 'pass').length
  const last = backups?.backups?.[0]
  return (
    <div className="space-y-3">
      {up ? (
        <ul aria-label={t('ov.statsLabel')} className="flex flex-wrap gap-2">
          <li>
            <button
              type="button"
              className={chipClass}
              onClick={() => onTab('domains')}
            >
              {report
                ? t('ov.chips.dns', { pass, total: items.length })
                : t('ov.chips.dnsNone')}
            </button>
          </li>
          <li>
            <button
              type="button"
              className={chipClass}
              onClick={() => onTab('backups')}
            >
              {last
                ? t('ov.chips.backup', {
                    when: new Date(last.created_at).toLocaleDateString(),
                  })
                : t('ov.chips.backupNone')}
            </button>
          </li>
          {queue && queue.queued > 0 ? (
            <li>
              <span className={chipClass}>
                {t('ov.chips.queue', { count: queue.queued })}
              </span>
            </li>
          ) : null}
        </ul>
      ) : null}
      {done ? (
        <div className="space-y-3">
          <div className="flex items-center gap-2 text-sm">
            <CircleCheckGlassIcon className="text-tone-success" size="1.3em" />
            <span className="font-medium">{t('ov.allSet')}</span>
            <Button
              size="xs"
              variant="ghost"
              aria-expanded={details}
              onClick={() => setDetails((v) => !v)}
            >
              {details ? t('ov.hideDetails') : t('ov.details')}
            </Button>
          </div>
          {details ? (
            <>
              <MailChecklist report={report} status={status} />
              <MailPortsCard />
            </>
          ) : null}
        </div>
      ) : (
        <>
          <MailNextStep
            status={status}
            report={report}
            domain={domain}
            onPreview={onPreview}
          />
          <MailChecklist report={report} status={status} />
          <MailPortsCard />
        </>
      )}
    </div>
  )
}
