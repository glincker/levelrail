import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import { aiControlQueryOptions, useAiControl } from '../../queries/aiControl'
import { AiControlCard } from '../../components/AiControlCard'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'
import { PageTemplate } from '@/components/shell/PageTemplate'

export const Route = createFileRoute('/settings/ai-control')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(aiControlQueryOptions()),
  component: AiControlPage,
  pendingComponent: AiControlSkeleton,
})

function AiControlSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <SettingsCardSkeleton rows={4} />
    </div>
  )
}

function AiControlPage() {
  const { t } = useTranslation('settings')
  const { data: settings } = useAiControl()

  return (
    <PageTemplate
      icon={<ShieldCheckIcon />}
      title={t('aiControl.title')}
      description={t('aiControl.description')}
    >
      <AiControlCard key={settings.updated_at} settings={settings} />
    </PageTemplate>
  )
}
