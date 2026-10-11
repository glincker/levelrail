import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  PlusIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { SkeletonLine, StatusPill, type Tone } from '@/components/kit'
import { Button } from '@/components/ui/button'
import type { AttentionLevel } from '../../lib/attentionSummary'
import { CreateResourceWizard } from '../CreateResourceWizard'
import { PageHeader } from '../shell/PageHeader'
import { useAttentionSummary } from '../shell/useAttentionSummary'

const LEVEL_TONE: Record<AttentionLevel, Tone> = {
  ok: 'success',
  warning: 'warning',
  critical: 'danger',
}

export function StatusHeader() {
  const { t } = useTranslation()
  const { t: td } = useTranslation('dashboard')
  const { level, count, isLoading } = useAttentionSummary()
  const label =
    level === 'ok' ? td('status.ok') : td('status.incidents', { count })
  const Icon = level === 'ok' ? CheckCircleIcon : WarningCircleIcon
  return (
    <PageHeader
      title={t('shell.dashboardTitle')}
      status={
        <>
          {isLoading ? (
            <SkeletonLine width={120} className="h-6 rounded-full" />
          ) : (
            <StatusPill
              tone={LEVEL_TONE[level]}
              label={label}
              live
              icon={
                <Icon weight="fill" className="size-4" aria-hidden="true" />
              }
            />
          )}
        </>
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
    />
  )
}
