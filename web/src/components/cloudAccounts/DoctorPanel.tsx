import { useMutation, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/ui/status-badge'
import type { StatusState } from '@/components/kit/tone'
import { apiRequest } from '../../lib/apiRequest'

type DoctorStatus = 'ok' | 'warn' | 'fail' | 'skip'

interface DoctorStep {
  id: string
  title: string
  status: DoctorStatus
  detail: string
  fix?: string
}

interface DoctorReport {
  provider?: string
  ready: boolean
  steps: DoctorStep[]
}

const STATE: Record<DoctorStatus, StatusState> = {
  ok: 'healthy',
  warn: 'degraded',
  fail: 'failed',
  skip: 'paused',
}

function useDeepDoctor(id: string) {
  return useMutation({
    mutationFn: () =>
      apiRequest<DoctorReport>(
        `/api/v1/cloud-accounts/${encodeURIComponent(id)}/doctor?deep=1`,
        { fallbackMessage: 'run deep check failed' },
      ),
  })
}

function useCloudAccountDoctor(id: string) {
  return useQuery({
    queryKey: ['cloud-accounts', 'doctor', id],
    queryFn: () =>
      apiRequest<DoctorReport>(
        `/api/v1/cloud-accounts/${encodeURIComponent(id)}/doctor`,
        { fallbackMessage: 'run readiness check failed' },
      ),
    staleTime: 0,
    refetchOnWindowFocus: false,
  })
}

export function DoctorPanel({
  id,
  compact = false,
}: Readonly<{ id: string; compact?: boolean }>) {
  const { t } = useTranslation('cloudAccounts')
  const doctor = useCloudAccountDoctor(id)
  const deep = useDeepDoctor(id)
  // The deep run repeats the readiness steps, so show its list instead of the base one.
  const report = deep.data ?? doctor.data
  const steps = report?.steps ?? []
  // Compact mode lists only what needs attention so the accounts list stays short.
  const shown = compact ? steps.filter((s) => s.status !== 'ok') : steps
  return (
    <section
      aria-label={t('doctor.title')}
      className="space-y-2 rounded-md border border-border p-3"
    >
      <div className="flex items-center gap-2">
        <h3 className="flex-1 text-sm font-medium">{t('doctor.title')}</h3>
        {report ? (
          <StatusBadge
            state={report.ready ? 'healthy' : 'failed'}
            label={report.ready ? t('doctor.ready') : t('doctor.notReady')}
          />
        ) : null}
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={doctor.isFetching}
          onClick={() => void doctor.refetch()}
        >
          {doctor.isFetching ? t('doctor.running') : t('doctor.rerun')}
        </Button>
        {compact ? null : (
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={deep.isPending}
            onClick={() => deep.mutate()}
          >
            {deep.isPending ? t('doctor.deepRunning') : t('doctor.deep')}
          </Button>
        )}
      </div>
      {doctor.isError || deep.isError ? (
        <p role="alert" className="text-xs text-destructive">
          {t('doctor.failed')}
        </p>
      ) : null}
      <ul className="space-y-1.5">
        {shown.map((s) => (
          <li key={s.id} className="flex flex-wrap items-start gap-2 text-xs">
            <StatusBadge
              state={STATE[s.status]}
              label={t(`doctor.status.${s.status}`)}
            />
            <div className="min-w-0 flex-1">
              <p className="font-medium text-foreground">{s.title}</p>
              <p className="text-muted-foreground">{s.detail}</p>
              {s.fix && s.status !== 'ok' ? (
                <p className="text-foreground">
                  {t('doctor.fix')}: {s.fix}
                </p>
              ) : null}
            </div>
          </li>
        ))}
      </ul>
    </section>
  )
}
