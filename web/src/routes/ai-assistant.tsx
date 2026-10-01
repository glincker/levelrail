import { requireExperimental } from '../lib/experimental'
import { createFileRoute, Link } from '@tanstack/react-router'
import { GearIcon, RobotIcon } from '@phosphor-icons/react/dist/ssr'
import {
  aiAssistantSettingsQueryOptions,
  useAiAssistantSettings,
} from '../queries/aiAssistantSettings'
import { AiChatPanel } from '../components/AiChatPanel'
import { PageHeader } from '../components/shell/PageHeader'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'

export const Route = createFileRoute('/ai-assistant')({
  beforeLoad: ({ context: { queryClient } }) =>
    requireExperimental(queryClient, 'ai-chat'),
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(aiAssistantSettingsQueryOptions()),
  component: AiAssistantPage,
  pendingComponent: AiAssistantSkeleton,
})

function AiAssistantPage() {
  const { data: settings } = useAiAssistantSettings()

  return (
    <div className="flex h-[calc(100vh-8rem)] flex-col gap-4">
      <PageHeader
        title="AI Assistant"
        description="Reads logs, metrics, and deploy history automatically. Always asks before deploying, rolling back, or restarting anything."
      />

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

// The chat transcript itself isn't worth faking message-by-message, so
// this mirrors just the outer chrome: header, a flex-1 body, composer bar.
function AiAssistantSkeleton() {
  return (
    <div
      className="flex h-[calc(100vh-8rem)] flex-col gap-4"
      aria-hidden="true"
    >
      <div className="space-y-2">
        <Skeleton className="h-6 w-32" />
        <Skeleton className="h-4 w-80" />
      </div>
      <div className="flex-1 rounded-lg border border-border" />
      <Skeleton className="h-16 w-full rounded-md" />
    </div>
  )
}
