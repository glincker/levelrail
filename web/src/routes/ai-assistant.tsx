import { createFileRoute, Link } from '@tanstack/react-router'
import { GearIcon, RobotIcon } from '@phosphor-icons/react/dist/ssr'
import {
  aiAssistantSettingsQueryOptions,
  useAiAssistantSettings,
} from '../queries/aiAssistantSettings'
import { AiChatPanel } from '../components/AiChatPanel'
import { PageSpinner } from '../components/ui/page-spinner'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'

export const Route = createFileRoute('/ai-assistant')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(aiAssistantSettingsQueryOptions()),
  component: AiAssistantPage,
  pendingComponent: PageSpinner,
})

function AiAssistantPage() {
  const { data: settings } = useAiAssistantSettings()

  return (
    <div className="flex h-[calc(100vh-8rem)] flex-col gap-4">
      <div>
        <h1 className="text-lg font-semibold text-foreground">AI Assistant</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Reads logs, metrics, and deploy history automatically. Always asks
          before deploying, rolling back, or restarting anything.
        </p>
      </div>

      {settings.configured ? (
        <AiChatPanel />
      ) : (
        <EmptyState
          icon={<RobotIcon className="size-5" />}
          title="No AI provider configured"
          description="Add a BYOK API key in Settings to start chatting."
          action={
            <Button size="sm" render={<Link to="/settings/ai-assistant" />}>
              <GearIcon />
              Configure AI Assistant
            </Button>
          }
          className="flex-1"
        />
      )}
    </div>
  )
}
