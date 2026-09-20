// AI assistant chat sessions: POST /api/v1/ai/sessions, GET
// /api/v1/ai/sessions/{id}, POST /api/v1/ai/sessions/{id}/messages (SSE),
// POST /api/v1/ai/sessions/{id}/confirmations/{confirmationId} (SSE).
//
// The messages and confirmations endpoints stream Server-Sent Events
// over a POST response body, which the browser's EventSource cannot
// consume (it only ever issues GET). hooks/useLogStream.ts's EventSource
// pattern doesn't apply here for that reason; streamAiSseResponse below
// reads the fetch Response body directly instead. Event types:
// text_delta (append to the in-flight assistant message), tool_call_proposed
// (a read-only tool that already auto-executed, or a mutating one
// pending confirmation), tool_result (the outcome of an executed tool,
// matched back to its tool_call_proposed by confirmation_id), and done
// (stream end).
//
// Session state itself isn't run through TanStack Query's cache, the
// same reasoning queries/deployLogs.ts documents for its own live
// append-only stream: hooks/useAiChatSession.ts owns the message list
// directly instead.

import { ApiError, readErrorMessage } from '../lib/apiError'
import type { AiChatSession, AiToolCall } from '../types/aiAssistant'

export interface AiSessionSummary {
  id: string
}

export async function createAiSession(): Promise<AiSessionSummary> {
  const res = await fetch('/api/v1/ai/sessions', { method: 'POST' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create AI session failed: ${res.status}`),
    )
  }
  return (await res.json()) as AiSessionSummary
}

export async function fetchAiSession(id: string): Promise<AiChatSession> {
  const res = await fetch(`/api/v1/ai/sessions/${encodeURIComponent(id)}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch AI session failed: ${res.status}`),
    )
  }
  return (await res.json()) as AiChatSession
}

export type AiSseEvent =
  | { type: 'text_delta'; content: string }
  | { type: 'tool_call_proposed'; toolCall: AiToolCall }
  | {
      type: 'tool_result'
      confirmationId: string
      result: unknown
      isError: boolean
    }
  | { type: 'done' }

function readString(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function readRecord(value: unknown): Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}
}

// The exact tool-call-argument key (`arguments` vs `args`) and the
// read-only flag's polarity (`read_only` vs `mutating`) aren't pinned
// down any further than "the tool name/args" and "whether it's read-only
// or mutating" in the fixed contract, so both spellings are accepted
// here. Unknown polarity defaults to treating the call as mutating
// (requires confirmation): the locked scope decision is that no
// mutating action ever runs unconfirmed, so an unrecognized shape must
// fail toward asking, never toward silently skipping the confirm step.
function parseToolCallProposed(data: Record<string, unknown>): AiToolCall {
  const argsValue = 'arguments' in data ? data.arguments : data.args
  let readOnly = false
  if (typeof data.read_only === 'boolean') {
    readOnly = data.read_only
  } else if (typeof data.mutating === 'boolean') {
    readOnly = !data.mutating
  }
  return {
    confirmation_id: readString(data.confirmation_id),
    tool_name: readString(data.tool_name ?? data.name),
    arguments: readRecord(argsValue),
    read_only: readOnly,
  }
}

function parseAiSseEvent(eventName: string, data: unknown): AiSseEvent | null {
  const record = readRecord(data)
  switch (eventName) {
    case 'text_delta':
      return {
        type: 'text_delta',
        content: readString(record.content ?? record.text ?? data),
      }
    case 'tool_call_proposed':
      return {
        type: 'tool_call_proposed',
        toolCall: parseToolCallProposed(record),
      }
    case 'tool_result':
      return {
        type: 'tool_result',
        confirmationId: readString(record.confirmation_id),
        result: 'result' in record ? record.result : data,
        isError: Boolean(record.is_error),
      }
    case 'done':
      return { type: 'done' }
    default:
      return null
  }
}

// Splits an SSE byte stream on blank-line frame boundaries and parses
// each frame's `event:`/`data:` fields, JSON-decoding `data`. A frame
// with no `event:` line is ignored (every event this feature cares about
// is always named, unlike the bare-line log stream in useLogStream.ts).
export async function streamAiSseResponse(
  res: Response,
  onEvent: (event: AiSseEvent) => void,
): Promise<void> {
  if (!res.body) return
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  const consumeFrame = (frame: string) => {
    let eventName = ''
    const dataLines: string[] = []
    for (const rawLine of frame.split('\n')) {
      const line = rawLine.endsWith('\r') ? rawLine.slice(0, -1) : rawLine
      if (line.startsWith('event:')) {
        eventName = line.slice('event:'.length).trim()
      } else if (line.startsWith('data:')) {
        dataLines.push(line.slice('data:'.length).trimStart())
      }
    }
    if (!eventName) return
    const raw = dataLines.join('\n')
    let data: unknown = undefined
    if (raw) {
      try {
        data = JSON.parse(raw)
      } catch {
        data = raw
      }
    }
    const parsed = parseAiSseEvent(eventName, data)
    if (parsed) onEvent(parsed)
  }

  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let sepIndex = buffer.indexOf('\n\n')
    while (sepIndex !== -1) {
      consumeFrame(buffer.slice(0, sepIndex))
      buffer = buffer.slice(sepIndex + 2)
      sepIndex = buffer.indexOf('\n\n')
    }
  }
  if (buffer.trim()) consumeFrame(buffer)
}

export async function sendAiChatMessage(
  sessionId: string,
  content: string,
  onEvent: (event: AiSseEvent) => void,
): Promise<void> {
  const res = await fetch(
    `/api/v1/ai/sessions/${encodeURIComponent(sessionId)}/messages`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'text/event-stream',
      },
      body: JSON.stringify({ content }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `send message failed: ${res.status}`),
    )
  }
  await streamAiSseResponse(res, onEvent)
}

export async function resolveAiToolConfirmation(
  sessionId: string,
  confirmationId: string,
  approve: boolean,
  onEvent: (event: AiSseEvent) => void,
): Promise<void> {
  const res = await fetch(
    `/api/v1/ai/sessions/${encodeURIComponent(sessionId)}/confirmations/${encodeURIComponent(confirmationId)}`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'text/event-stream',
      },
      body: JSON.stringify({ approve }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `resolve confirmation failed: ${res.status}`),
    )
  }
  await streamAiSseResponse(res, onEvent)
}
