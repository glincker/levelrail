import { createFileRoute } from '@tanstack/react-router'
import {
  aiAssistantSettingsQueryOptions,
  useAiAssistantSettings,
} from '../../queries/aiAssistantSettings'
import { AiAssistantSettingsCard } from '../../components/AiAssistantSettingsCard'
import { PageSpinner } from '../../components/ui/page-spinner'

export const Route = createFileRoute('/settings/ai-assistant')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(aiAssistantSettingsQueryOptions()),
  component: AiAssistantSettingsPage,
  pendingComponent: PageSpinner,
})

function AiAssistantSettingsPage() {
  const { data: settings } = useAiAssistantSettings()

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">AI Assistant</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Bring your own LLM API key to power the chat assistant.
        </p>
      </div>

      <AiAssistantSettingsCard settings={settings} />
    </div>
  )
}
