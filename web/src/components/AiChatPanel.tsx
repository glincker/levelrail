import {
  ArrowClockwiseIcon,
  ChatCircleTextIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useAiChatSession } from '../hooks/useAiChatSession'
import { AiChatComposer } from './AiChatComposer'
import { AiChatMessageList } from './AiChatMessageList'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'

// Owns one AI assistant conversation: session lifecycle, message
// transcript, and the composer, wired to useAiChatSession which does
// the actual SSE plumbing. Rendered once the caller has already
// confirmed a BYOK key is configured (routes/ai-assistant.tsx).
export function AiChatPanel() {
  const {
    sessionId,
    messages,
    connectionState,
    streaming,
    error,
    sendMessage,
    resolveConfirmation,
    startNewSession,
  } = useAiChatSession()

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <div className="flex items-center justify-end">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={connectionState === 'connecting'}
          onClick={startNewSession}
        >
          <ArrowClockwiseIcon />
          New chat
        </Button>
      </div>

      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      {connectionState === 'connecting' && messages.length === 0 ? (
        <div className="flex-1" />
      ) : messages.length === 0 ? (
        <EmptyState
          icon={<ChatCircleTextIcon className="size-5" />}
          title="Ask about your platform"
          description="Logs, metrics, deploy history, and crashloop diagnosis are read automatically. Anything that changes state (deploy, rollback, restart) always waits for your approval first."
          className="flex-1"
        />
      ) : (
        <AiChatMessageList
          messages={messages}
          streaming={streaming}
          onApproveToolCall={(confirmationId) => {
            void resolveConfirmation(confirmationId, true)
          }}
          onRejectToolCall={(confirmationId) => {
            void resolveConfirmation(confirmationId, false)
          }}
        />
      )}

      <AiChatComposer
        onSend={(content) => {
          void sendMessage(content)
        }}
        disabled={!sessionId || streaming}
      />
    </div>
  )
}
