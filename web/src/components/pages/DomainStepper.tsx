import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon } from '@thesvg/ui-react'
import { domainSteps, DOMAIN_STEPS as STEPS } from './pageHelpers'

export function DomainStepper({
  dnsOk,
  certOk,
}: Readonly<{ dnsOk: boolean; certOk: boolean }>) {
  const { t } = useTranslation('pagesSites')
  const state = domainSteps(dnsOk, certOk)
  return (
    <ol
      aria-label={t('domains.progress')}
      className="flex items-center gap-1.5 text-xs"
    >
      {STEPS.map((k, i) => (
        <li key={k} className="flex items-center gap-1.5">
          {i > 0 ? (
            <span
              aria-hidden="true"
              className={`h-px w-4 transition-colors duration-200 ${state[k] === 'done' ? 'bg-success' : 'bg-border'}`}
            />
          ) : null}
          {state[k] === 'done' ? (
            <CircleCheckGlassIcon
              className="size-3.5 text-success"
              aria-hidden="true"
            />
          ) : (
            <span
              aria-hidden="true"
              className="size-3 rounded-full border border-muted-foreground/60"
            />
          )}
          <span
            className={
              state[k] === 'done' ? 'text-foreground' : 'text-muted-foreground'
            }
          >
            {t(`domains.step.${k}`)}
            <span className="sr-only">
              {' '}
              {state[k] === 'done' ? t('domains.done') : t('domains.waiting')}
            </span>
          </span>
        </li>
      ))}
    </ol>
  )
}
