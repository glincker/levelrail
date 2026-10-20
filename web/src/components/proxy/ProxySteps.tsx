import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  CircleIcon,
  ProhibitIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import type { ProxyStep, ProxyStepState } from '../../queries/proxyIntegration'

const ICONS = {
  done: { Icon: CheckCircleIcon, tone: 'text-green-600 dark:text-green-400' },
  todo: { Icon: CircleIcon, tone: 'text-muted-foreground' },
  blocked: { Icon: ProhibitIcon, tone: 'text-amber-600 dark:text-amber-400' },
  error: { Icon: XCircleIcon, tone: 'text-destructive' },
} as const satisfies Record<ProxyStepState, { Icon: unknown; tone: string }>

export function ProxySteps({ steps }: { steps: ProxyStep[] }) {
  const { t } = useTranslation('domains')
  return (
    <ol className="space-y-0" aria-label={t('proxySetup.title')}>
      {steps.map((step, i) => {
        const { Icon, tone } = ICONS[step.state]
        const last = i === steps.length - 1
        return (
          <li key={step.id} className="relative flex gap-3 pb-4 last:pb-0">
            {last ? null : (
              <span
                aria-hidden="true"
                className="absolute top-6 bottom-0 left-2.75 w-px bg-border"
              />
            )}
            <Icon
              weight="fill"
              className={cn('relative size-5.5 shrink-0 bg-card', tone)}
              aria-hidden="true"
            />
            <div className="min-w-0">
              <p className="text-sm font-medium text-foreground">
                {t(`proxySetup.steps.${step.id}`)}
                <span className="sr-only">
                  {' '}
                  ({t(`proxySetup.stepState.${step.state}`)})
                </span>
              </p>
              {step.detail ? (
                <p className="text-xs text-muted-foreground">{step.detail}</p>
              ) : null}
            </div>
          </li>
        )
      })}
    </ol>
  )
}
