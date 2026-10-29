import { useTranslation } from 'react-i18next'
import {
  ActivityGlassIcon,
  BoxGlassIcon,
  RocketGlassIcon,
  UploadGlassIcon,
} from '@thesvg/ui-react'
import { cardAnchor } from './groups'

const STEPS = [
  { key: 'build', href: `#${cardAnchor('buildCache')}`, Icon: BoxGlassIcon },
  { key: 'push', href: '#build', Icon: UploadGlassIcon },
  {
    key: 'rollout',
    href: `#${cardAnchor('strategy')}`,
    Icon: RocketGlassIcon,
  },
  {
    key: 'healthCheck',
    href: `#${cardAnchor('safety')}`,
    Icon: ActivityGlassIcon,
  },
] as const

export function DeployFlowStrip() {
  const { t } = useTranslation('common')
  return (
    <section aria-labelledby="deploy-flow-title" className="space-y-2">
      <h2
        id="deploy-flow-title"
        className="text-sm font-semibold text-foreground"
      >
        {t('deploySettings.flow.title')}
      </h2>
      <ol className="grid gap-2 sm:grid-cols-4">
        {STEPS.map(({ key, href, Icon }, i) => (
          <li key={key}>
            <a
              href={href}
              className="flex h-full flex-col gap-1 rounded-lg border border-border bg-card p-3 transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none motion-reduce:transition-none"
            >
              <span className="flex items-center gap-2 text-sm font-medium text-foreground">
                <Icon aria-hidden="true" className="size-5" />
                {i + 1}. {t(`deploySettings.flow.${key}`)}
              </span>
              <span className="text-xs text-muted-foreground">
                {t(`deploySettings.flow.${key}Hint`)}
              </span>
            </a>
          </li>
        ))}
      </ol>
    </section>
  )
}
