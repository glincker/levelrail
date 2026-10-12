import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  PlusIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { SkeletonLine, StatusPill, type Tone } from '@/components/kit'
import { Button } from '@/components/ui/button'
import type { AppListEntry } from '../../types/appDetail'
import type { AttentionLevel } from '../../lib/attentionSummary'
import { countBuckets } from '../../lib/appsListView'
import { dayPeriod, heroSentence } from '../../lib/dashboardHome'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { useBrand } from '../../hooks/useBrand'
import { useMinuteNow } from '../../hooks/useMinuteNow'
import { CreateResourceWizard } from '../CreateResourceWizard'
import { PageTemplate } from '../shell/PageTemplate'
import { useAttentionSummary } from '../shell/useAttentionSummary'

const LEVEL_TONE: Record<AttentionLevel, Tone> = {
  ok: 'success',
  warning: 'warning',
  critical: 'danger',
}

export function DashboardHero({
  apps,
  banners,
  children,
}: {
  apps: AppListEntry[]
  banners?: ReactNode
  children: ReactNode
}) {
  const { t } = useTranslation()
  const { t: td } = useTranslation('dashboard')
  const brand = useBrand()
  const username = useAuthUsername()
  const now = useMinuteNow()
  const { level, count, isLoading } = useAttentionSummary()
  const failing = countBuckets(apps).failing
  const sentence = heroSentence({
    apps: apps.length,
    failing,
    attention: isLoading ? 0 : count,
  })
  const period = dayPeriod(new Date(now).getHours())
  const greeting = username
    ? td('hero.greetingNamed', { context: period, name: username })
    : td('hero.greeting', { context: period })
  const label =
    level === 'ok' ? td('status.ok') : td('status.incidents', { count })
  const Icon = level === 'ok' ? CheckCircleIcon : WarningCircleIcon

  return (
    <PageTemplate
      width="wide"
      eyebrow={brand.Name}
      title={greeting}
      description={td(`hero.sentence.${sentence.key}`, {
        count: sentence.count,
      })}
      status={
        isLoading ? (
          <SkeletonLine width={120} className="h-6 rounded-full" />
        ) : (
          <StatusPill
            tone={LEVEL_TONE[level]}
            label={label}
            live
            icon={<Icon weight="fill" className="size-4" aria-hidden="true" />}
          />
        )
      }
      actions={
        <CreateResourceWizard
          trigger={
            <Button size="sm">
              <PlusIcon />
              {t('shell.newApp')}
            </Button>
          }
        />
      }
      banners={banners}
    >
      {children}
    </PageTemplate>
  )
}
