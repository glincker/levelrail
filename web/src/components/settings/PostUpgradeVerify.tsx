import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, CircleXGlassIcon } from '@thesvg/ui-react'
import { StatusPill, type Tone } from '@/components/kit'
import { Button } from '../ui/button'
import {
  doctorQueryOptions,
  smokeQueryOptions,
  type DoctorCheck,
  type SmokeStatus,
} from '../../queries/releases'

const TONE: Record<DoctorCheck['status'], Tone> = {
  ok: 'success',
  warn: 'warning',
  fail: 'danger',
}

const SMOKE_TONE: Record<SmokeStatus, Tone> = {
  pass: 'success',
  warn: 'warning',
  fail: 'danger',
  skipped: 'neutral',
}

function SmokeTour() {
  const { t } = useTranslation('updates')
  const { data, isFetching, isError, error, refetch } =
    useQuery(smokeQueryOptions())
  return (
    <div className="space-y-2">
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={isFetching}
        onClick={() => {
          void refetch()
        }}
      >
        {isFetching ? t('smoke.running') : t('smoke.run')}
      </Button>
      {isError ? (
        <p role="alert" className="text-sm text-destructive">
          {t('smoke.error', { message: error.message })}
        </p>
      ) : null}
      {data ? (
        <ul className="space-y-1.5" aria-label={t('smoke.title')}>
          {data.items.map((item) => (
            <li key={item.id} className="flex items-start gap-3 text-sm">
              <StatusPill
                tone={SMOKE_TONE[item.status]}
                label={t(`smoke.status.${item.status}`)}
                size="sm"
              />
              <span>
                <span className="font-medium text-foreground">
                  {t(`smoke.item.${item.id}`)}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {item.message}
                </span>
              </span>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}

export function PostUpgradeVerify() {
  const { t } = useTranslation('updates')
  const { data, isFetching, isError, error, refetch, dataUpdatedAt } =
    useQuery(doctorQueryOptions())
  const failed = data ? data.checks.filter((c) => c.status === 'fail') : []
  const warned = data ? data.checks.filter((c) => c.status === 'warn') : []

  return (
    <div className="space-y-3">
      <SmokeTour />
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={isFetching}
        onClick={() => {
          void refetch()
        }}
      >
        {isFetching ? t('verify.running') : t('verify.run')}
      </Button>
      {isError ? (
        <p role="alert" className="text-sm text-destructive">
          {t('verify.error', { message: error.message })}
        </p>
      ) : null}
      {data ? (
        <div className="space-y-2" role="status">
          <p
            className={
              data.ok
                ? 'inline-flex items-center gap-1.5 text-sm text-tone-success'
                : 'inline-flex items-center gap-1.5 text-sm text-destructive'
            }
          >
            {data.ok ? (
              <CircleCheckGlassIcon className="size-5 text-tone-success" />
            ) : (
              <CircleXGlassIcon className="size-5 text-tone-danger" />
            )}
            {data.ok
              ? warned.length > 0
                ? t('verify.passWarn')
                : t('verify.pass')
              : failed.length === 1
                ? t('verify.fail', { count: failed.length })
                : t('verify.failPlural', { count: failed.length })}
          </p>
          <ul className="space-y-1.5">
            {data.checks
              .filter((c) => c.status !== 'ok')
              .map((c) => (
                <li key={c.code} className="flex items-start gap-3 text-sm">
                  <StatusPill
                    tone={TONE[c.status]}
                    label={t(`preflight.status.${c.status}`)}
                    size="sm"
                  />
                  <span>
                    <span className="font-medium text-foreground">
                      {c.name}
                    </span>
                    <span className="block text-xs text-muted-foreground">
                      {c.message}
                    </span>
                  </span>
                </li>
              ))}
          </ul>
          <p className="text-xs text-muted-foreground">
            {t('verify.checkedAt', {
              time: new Date(dataUpdatedAt).toLocaleTimeString(),
            })}
          </p>
        </div>
      ) : null}
    </div>
  )
}
