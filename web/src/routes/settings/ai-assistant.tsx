import { requireExperimental } from '../../lib/experimental'
import { createFileRoute } from '@tanstack/react-router'
import { RobotIcon } from '@phosphor-icons/react/dist/ssr'
import {
  aiAssistantSettingsQueryOptions,
  useAiAssistantSettings,
} from '../../queries/aiAssistantSettings'
import { AiAssistantSettingsCard } from '../../components/AiAssistantSettingsCard'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'
import { PageHeader } from '@/components/shell/PageHeader'

export const Route = createFileRoute('/settings/ai-assistant')({
  beforeLoad: ({ context: { queryClient } }) =>
    requireExperimental(queryClient, 'ai-chat'),
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
  const { data: settings } = useAiAssistantSettings()

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <RobotIcon className="size-4" />
        </div>
        <PageHeader
          title="AI Assistant"
          description="Bring your own LLM API key to power the chat assistant."
        />
      </div>

      <AiAssistantSettingsCard settings={settings} />
    </div>
  )
}
