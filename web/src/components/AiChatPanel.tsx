import { Suspense, useEffect, useRef } from 'react'
import {
  ArrowClockwiseIcon,
  ChatCircleTextIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useAiChatSession } from '../hooks/useAiChatSession'
import { takeAiChatSeed } from '../lib/aiChatSeed'
import { AiChatComposer } from './AiChatComposer'
import { AiChatHistoryMenu } from './AiChatHistoryMenu'
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
    resumeSession,
  } = useAiChatSession()

  // Consumes a seed queued by a dashboard "Ask AI" click (see
  // lib/aiChatSeed.ts) the moment the first session is ready, so the
  // assistant opens already holding the specific item's context.
  const seededRef = useRef(false)
  useEffect(() => {
    if (seededRef.current) return
    if (connectionState !== 'ready' || !sessionId) return
    seededRef.current = true
    const seed = takeAiChatSeed()
    if (seed) void sendMessage(seed)
  }, [connectionState, sessionId, sendMessage])

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <div className="flex items-center justify-end gap-2">
        <Suspense fallback={null}>
          <AiChatHistoryMenu
            activeSessionId={sessionId}
            onResume={resumeSession}
          />
        </Suspense>
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
