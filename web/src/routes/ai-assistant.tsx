import { requireExperimental } from '../lib/experimental'
import { createFileRoute, Link } from '@tanstack/react-router'
import { BotGlassIcon, SettingsGlassIcon } from '@thesvg/ui-react'
import {
  aiAssistantSettingsQueryOptions,
  useAiAssistantSettings,
} from '../queries/aiAssistantSettings'
import { AiChatPanel } from '../components/AiChatPanel'
import { DemoGate } from '@/components/demo/DemoUnavailable'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/shell/PageHeader'

export const Route = createFileRoute('/ai-assistant')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'ai-chat', location.href),
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

      <DemoGate reason="generic">
        {settings.configured ? (
          <AiChatPanel />
        ) : (
          <EmptyState
            icon={<BotGlassIcon className="size-6" />}
            title="No AI provider configured"
            description="Add a BYOK API key in Settings to start chatting."
            action={
              <Button size="sm" render={<Link to="/settings/ai-assistant" />}>
                <SettingsGlassIcon size="1.4em" />
                Configure AI Assistant
              </Button>
            }
            className="flex-1"
          />
        )}
      </DemoGate>
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
