import { useCallback, useEffect, useRef, useState } from 'react'
import {
  createAiSession,
  fetchAiSession,
  resolveAiToolConfirmation,
  sendAiChatMessage,
  type AiSseEvent,
} from '../queries/aiAssistant'
import type { AiChatMessage } from '../types/aiAssistant'

export type AiChatConnectionState = 'idle' | 'connecting' | 'ready' | 'error'

function newLocalMessage(
  role: 'user' | 'assistant',
  content: string,
): AiChatMessage {
  return {
    id: `local-${role}-${crypto.randomUUID()}`,
    role,
    content,
    tool_calls: role === 'assistant' ? [] : null,
    created_at: new Date().toISOString(),
  }
}

// Applies one SSE event onto the tail of the message list. text_delta
// and tool_call_proposed extend the most recent assistant message in
// place; tool_result is matched back to that message's pending tool
// call by confirmation_id. Both sendMessage (starts a fresh assistant
// message) and resolveConfirmation ("continues the same turn" per the
// fixed API contract) rely on the last message already being that
// in-flight assistant message when their event stream starts.
function applyAiSseEvent(
  messages: AiChatMessage[],
  event: AiSseEvent,
): AiChatMessage[] {
  if (event.type === 'done') return messages

  const last = messages[messages.length - 1]
  const target: AiChatMessage =
    last?.role === 'assistant' ? last : newLocalMessage('assistant', '')

  let updated: AiChatMessage
  if (event.type === 'text_delta') {
    updated = { ...target, content: target.content + event.content }
  } else if (event.type === 'tool_call_proposed') {
    updated = {
      ...target,
      tool_calls: [...(target.tool_calls ?? []), event.toolCall],
    }
  } else {
    updated = {
      ...target,
      tool_calls: (target.tool_calls ?? []).map((call) =>
        call.confirmation_id === event.confirmationId
          ? { ...call, result: event.result, is_error: event.isError }
          : call,
      ),
    }
  }

  const base = last?.role === 'assistant' ? messages.slice(0, -1) : messages
  return [...base, updated]
}

// Owns one AI assistant chat session end to end: creates it on mount,
// hydrates any existing transcript, and exposes sendMessage/
// resolveConfirmation, both of which stream SSE events directly into
// the local message list (see queries/aiAssistant.ts's own doc comment
// for why this bypasses TanStack Query's cache).
export function useAiChatSession() {
  const [sessionId, setSessionId] = useState<string | null>(null)
  const [messages, setMessages] = useState<AiChatMessage[]>([])
  const [connectionState, setConnectionState] =
    useState<AiChatConnectionState>('idle')
  const [streaming, setStreaming] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const startingRef = useRef(false)

  const startSession = useCallback(() => {
    if (startingRef.current) return
    startingRef.current = true
    setConnectionState('connecting')
    setError(null)
    setMessages([])
    createAiSession()
      .then((session) => {
        setSessionId(session.id)
        setConnectionState('ready')
      })
      .catch((err: unknown) => {
        setConnectionState('error')
        setError(
          err instanceof Error ? err.message : 'Failed to start a chat session',
        )
      })
      .finally(() => {
        startingRef.current = false
      })
  }, [])

  useEffect(() => {
    startSession()
    // Intentionally runs once: startSession's own ref guard, not this
    // effect's dependency array, is what prevents a double session
    // create under StrictMode's dev double-invoke.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (!sessionId) return
    let cancelled = false
    fetchAiSession(sessionId)
      .then((session) => {
        if (!cancelled) setMessages(session.messages)
      })
      .catch(() => {
        // A brand-new session has nothing to hydrate; a genuine fetch
        // failure here just leaves the locally-built transcript as is.
      })
    return () => {
      cancelled = true
    }
  }, [sessionId])

  const sendMessage = useCallback(
    async (content: string) => {
      const trimmed = content.trim()
      if (!sessionId || !trimmed || streaming) return
      setError(null)
      setMessages((prev) => [
        ...prev,
        newLocalMessage('user', trimmed),
        newLocalMessage('assistant', ''),
      ])
      setStreaming(true)
      try {
        await sendAiChatMessage(sessionId, trimmed, (event) => {
          setMessages((prev) => applyAiSseEvent(prev, event))
        })
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to send message')
      } finally {
        setStreaming(false)
      }
    },
    [sessionId, streaming],
  )

  const resolveConfirmation = useCallback(
    async (confirmationId: string, approve: boolean) => {
      if (!sessionId) return
      setMessages((prev) =>
        prev.map((message) => ({
          ...message,
          tool_calls:
            message.tool_calls?.map((call) =>
              call.confirmation_id === confirmationId
                ? { ...call, resolution: approve ? 'approved' : 'rejected' }
                : call,
            ) ?? message.tool_calls,
        })),
      )
      setError(null)
      setStreaming(true)
      try {
        await resolveAiToolConfirmation(
          sessionId,
          confirmationId,
          approve,
          (event) => {
            setMessages((prev) => applyAiSseEvent(prev, event))
          },
        )
      } catch (err) {
        setError(
          err instanceof Error ? err.message : 'Failed to resolve confirmation',
        )
      } finally {
        setStreaming(false)
      }
    },
    [sessionId],
  )

  return {
    sessionId,
    messages,
    connectionState,
    streaming,
    error,
    sendMessage,
    resolveConfirmation,
    startNewSession: startSession,
  }
}
