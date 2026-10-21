import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowsClockwiseIcon,
  CheckCircleIcon,
  HeartbeatIcon,
  InfoIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { HelpLink } from '@/components/HelpLink'
import { PageHeader } from '@/components/shell/PageHeader'
import { DoctorFixAll } from '@/components/doctor/DoctorFixAll'
import { DoctorGroupCard } from '@/components/doctor/DoctorGroupCard'
import { DoctorToolbar } from '@/components/doctor/DoctorToolbar'
import {
  buildDiagnostics,
  filterGroups,
  type DoctorFilter,
} from '../../lib/doctorDiagnostics'
import { DiskResilienceCard } from '../../components/DiskResilienceCard'
import { ResilienceReportCard } from '../../components/ResilienceReportCard'
import { useDoctorStream } from '../../hooks/useDoctorStream'
import { allChecks, countSeverity, groupChecks } from '../../lib/doctorModel'

// The one Doctor: the same registered checks the CLI's doctor command runs,
// streamed so the page paints as each check finishes.
export const Route = createFileRoute('/settings/system-status')({
  component: DoctorPage,
})

function SummaryBanner({
  fail,
  warn,
  running,
}: {
  fail: number
  warn: number
  running: boolean
}) {
  const { t } = useTranslation('settings')
  if (running) return null
  if (fail === 0 && warn === 0) {
    return (
      <Alert>
        <CheckCircleIcon />
        <AlertTitle>{t('doctorPage.allPassedTitle')}</AlertTitle>
        <AlertDescription>{t('doctorPage.allPassedBody')}</AlertDescription>
      </Alert>
    )
  }
  return (
    <Alert variant={fail > 0 ? 'destructive' : 'default'}>
      <XCircleIcon />
      <AlertTitle>
        {fail > 0
          ? t('doctorPage.failedTitle', { count: fail })
          : t('doctorPage.warnTitle', { count: warn })}
      </AlertTitle>
      <AlertDescription>{t('doctorPage.reviewBelow')}</AlertDescription>
    </Alert>
  )
}

function DoctorPage() {
  const { t } = useTranslation('settings')
  const { state, rerun, recheck } = useDoctorStream()
  const checks = allChecks(state)
  const counts = countSeverity(checks)
  const [filter, setFilter] = useState<DoctorFilter>('all')
  const [checkedAt, setCheckedAt] = useState<Date | null>(null)
  const groups = filterGroups(groupChecks(checks), filter)

  const [wasRunning, setWasRunning] = useState(state.running)
  if (wasRunning !== state.running) {
    setWasRunning(state.running)
    if (!state.running && state.total > 0) setCheckedAt(new Date())
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <HeartbeatIcon className="size-4" />
        </div>
        <div className="min-w-0 flex-1">
          <PageHeader
            title={t('doctorPage.title')}
            description={t('doctorPage.description')}
            helpPath="/doctor"
            helpLabel={t('doctorPage.guide')}
            actions={
              <Button
                variant="outline"
                size="sm"
                onClick={rerun}
                disabled={state.running}
              >
                <ArrowsClockwiseIcon
                  className={state.running ? 'animate-spin' : ''}
                />
                {t('doctorPage.rerun')}
              </Button>
            }
          />
        </div>
      </div>

      <DoctorToolbar
        counts={counts}
        total={checks.length}
        filter={filter}
        onFilter={setFilter}
        checkedAt={checkedAt}
        running={state.running}
        diagnostics={() => buildDiagnostics(checks, checkedAt)}
      />
      {state.running ? (
        <p className="text-xs text-muted-foreground" aria-live="polite">
          {t('doctorPage.progress', {
            done: state.finished,
            total: state.total,
          })}
        </p>
      ) : null}

      {state.error ? (
        <Alert variant="destructive">
          <XCircleIcon />
          <AlertTitle>{t('doctorPage.streamErrorTitle')}</AlertTitle>
          <AlertDescription>{t('doctorPage.streamErrorBody')}</AlertDescription>
        </Alert>
      ) : null}

      <SummaryBanner
        fail={counts.fail}
        warn={counts.warn}
        running={state.running}
      />
      {state.running ? null : <DoctorFixAll onDone={rerun} />}

      <Alert>
        <InfoIcon />
        <AlertTitle>{t('doctorPage.ingressTitle')}</AlertTitle>
        <AlertDescription>
          {t('doctorPage.ingressBody')}{' '}
          <HelpLink
            path="/resilience"
            label={t('doctorPage.survivesCrash')}
            variant="inline"
          />
          .
        </AlertDescription>
      </Alert>

      {groups.length === 0 && !state.running ? (
        <p className="text-sm text-muted-foreground">
          {t('doctorPage.noMatches')}
        </p>
      ) : null}
      {groups.map((g) => (
        <DoctorGroupCard key={g.id} group={g} recheck={recheck} />
      ))}
      <DiskResilienceCard />
      <ResilienceReportCard />
    </div>
  )
}
