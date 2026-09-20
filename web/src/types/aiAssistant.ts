// Wire types for the AI assistant chat feature: POST /api/v1/ai/sessions,
// GET /api/v1/ai/sessions/{id}, POST .../messages (SSE),
// POST .../confirmations/{confirmationId} (SSE). See queries/aiAssistant.ts
// for the fetchers and the SSE frame parser.

export type AiChatRole = 'user' | 'assistant' | 'tool'

export type AiToolCallResolution = 'approved' | 'rejected'

// A tool call proposed by the assistant mid-turn. read_only ones are
// auto-executed server-side (a tool_result event follows without any UI
// action); everything else stays pending until resolveConfirmation is
// called, per the locked scope decision that no mutating action ever
// runs without an explicit confirm step.
export interface AiToolCall {
  confirmation_id: string
  tool_name: string
  arguments: Record<string, unknown>
  read_only: boolean
  resolution?: AiToolCallResolution
  result?: unknown
  is_error?: boolean
}

export interface AiChatMessage {
  id: string
  role: AiChatRole
  content: string
  tool_calls: AiToolCall[] | null
  created_at: string
}

export interface AiChatSession {
  id: string
  messages: AiChatMessage[]
}
