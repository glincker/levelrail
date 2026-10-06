import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import { aiControlQueryOptions, useAiControl } from '../../queries/aiControl'
import { AiControlCard } from '../../components/AiControlCard'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'
import { PageHeader } from '@/components/shell/PageHeader'

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
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <ShieldCheckIcon className="size-4" />
        </div>
        <PageHeader
          title={t('aiControl.title')}
          description={t('aiControl.navDescription')}
        />
      </div>
      <AiControlCard key={settings.updated_at} settings={settings} />
    </div>
  )
}
