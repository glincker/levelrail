import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { firstRunSteps } from '../../lib/domainsFirstRun'

// Shown only while no domain exists: three steps in one row, the next one highlighted.
export function DomainsFirstRun({
  appCount,
  domainCount,
  httpsOff,
  onAdd,
}: {
  appCount: number
  domainCount: number
  httpsOff: boolean | undefined
  onAdd: () => void
}) {
  const { t } = useTranslation('domains')
  const steps = firstRunSteps({ appCount, domainCount, httpsOff })
  return (
    <ol aria-label={t('firstRun.label')} className="grid gap-3 sm:grid-cols-3">
      {steps.map((step, i) => (
        <li
          key={step.id}
          aria-current={step.current ? 'step' : undefined}
          className={cn(
            'flex min-w-0 flex-col gap-2 rounded-lg border bg-card p-4',
            step.current ? 'border-primary' : 'border-border',
            !step.current && !step.done && 'opacity-80',
          )}
        >
          <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <span
              className={cn(
                'flex size-5 items-center justify-center rounded-full text-xs',
                step.done
                  ? 'bg-tone-success/15 text-tone-success'
                  : step.current
                    ? 'bg-primary text-primary-foreground'
                    : 'bg-muted text-muted-foreground',
              )}
            >
              {step.done ? (
                <CheckGlassIcon size="1.2em" aria-hidden="true" />
              ) : (
                i + 1
              )}
            </span>
            {step.done
              ? t('firstRun.done')
              : step.current
                ? t('firstRun.current')
                : t('firstRun.step', { n: i + 1 })}
          </div>
          <h2 className="text-sm font-medium text-foreground">
            {t(`firstRun.${step.id}.title`)}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t(`firstRun.${step.id}.body`)}
          </p>
          {step.id === 'domain' ? (
            <Button
              type="button"
              size="sm"
              variant={step.current ? 'default' : 'outline'}
              disabled={appCount === 0}
              onClick={onAdd}
              className="mt-auto w-fit"
            >
              <PlusGlassIcon size="1.4em" />
              {t('firstRun.domain.action')}
            </Button>
          ) : null}
        </li>
      ))}
    </ol>
  )
}
