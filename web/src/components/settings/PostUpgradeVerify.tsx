import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CheckCircleIcon, XCircleIcon } from '@phosphor-icons/react/dist/ssr'
import { StatusPill, type Tone } from '@/components/kit'
import { Button } from '../ui/button'
import { doctorQueryOptions, type DoctorCheck } from '../../queries/releases'

const TONE: Record<DoctorCheck['status'], Tone> = {
  ok: 'success',
  warn: 'warning',
  fail: 'danger',
}

export function PostUpgradeVerify() {
  const { t } = useTranslation('updates')
  const { data, isFetching, isError, error, refetch, dataUpdatedAt } =
    useQuery(doctorQueryOptions())
  const failed = data ? data.checks.filter((c) => c.status === 'fail') : []
  const warned = data ? data.checks.filter((c) => c.status === 'warn') : []

  return (
    <div className="space-y-3">
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
                ? 'inline-flex items-center gap-1.5 text-sm text-green-700 dark:text-green-400'
                : 'inline-flex items-center gap-1.5 text-sm text-destructive'
            }
          >
            {data.ok ? (
              <CheckCircleIcon className="size-4" />
            ) : (
              <XCircleIcon className="size-4" />
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
