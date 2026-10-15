import { requireExperimental } from '../../lib/experimental'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  MinusCircleIcon,
  RobotIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  aiAssistantSettingsQueryOptions,
  useAiAssistantSettings,
} from '../../queries/aiAssistantSettings'
import { AiAssistantSettingsCard } from '../../components/AiAssistantSettingsCard'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { StatusBadge } from '@/components/ui/status-badge'

export const Route = createFileRoute('/settings/ai-assistant')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'ai-chat', location.href),
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(aiAssistantSettingsQueryOptions()),
  component: AiAssistantSettingsPage,
  pendingComponent: AiAssistantSettingsSkeleton,
})

function AiAssistantSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <SettingsCardSkeleton rows={3} />
    </div>
  )
}

function AiAssistantSettingsPage() {
  const { t } = useTranslation('settings')
  const { data: settings } = useAiAssistantSettings()

  return (
    <PageTemplate
      icon={<RobotIcon />}
      title={t('aiAssistantPage.title')}
      description={t('aiAssistantPage.description')}
      status={
        <StatusBadge
          variant={settings.configured ? 'success' : 'muted'}
          label={
            settings.configured
              ? t('aiAssistantPage.configured')
              : t('aiAssistantPage.notConfigured')
          }
          icon={settings.configured ? CheckCircleIcon : MinusCircleIcon}
        />
      }
    >
      <AiAssistantSettingsCard settings={settings} />
    </PageTemplate>
  )
}
