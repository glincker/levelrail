import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
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
      icon={<ShieldCheckGlassIcon className="text-tone-success" size="1.2em" />}
      title={t('aiControl.title')}
      description={t('aiControl.description')}
    >
      <AiControlCard key={settings.updated_at} settings={settings} />
    </PageTemplate>
  )
}
