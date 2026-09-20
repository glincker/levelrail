import { useEffect, useRef } from 'react'
import { RobotIcon, UserIcon, WrenchIcon } from '@phosphor-icons/react/dist/ssr'
import type { AiChatMessage } from '../types/aiAssistant'
import { AiToolCallCard } from './AiToolCallCard'
import { cn } from '@/lib/utils'

// Not virtualized: unlike the log viewer (hooks/useLogStream.ts's
// consumer), a chat transcript's per-message height is highly variable
// and the tail message is actively mutating mid-stream, so a
// react-virtual size cache would be invalidated on nearly every render
// while streaming. A conversation realistically stays in the tens of
// messages, not the thousands a virtualized resource list guards
// against.
export function AiChatMessageList({
  messages,
  streaming,
  onApproveToolCall,
  onRejectToolCall,
}: Readonly<{
  messages: AiChatMessage[]
  streaming: boolean
  onApproveToolCall: (confirmationId: string) => void
  onRejectToolCall: (confirmationId: string) => void
}>) {
  const bottomRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    // jsdom (component tests) has no scrollIntoView implementation.
    bottomRef.current?.scrollIntoView?.({ block: 'end' })
  }, [messages, streaming])

  return (
    <div className="flex-1 space-y-3 overflow-y-auto pr-1">
      {messages.map((message) => (
        <AiChatMessageRow
          key={message.id}
          message={message}
          onApproveToolCall={onApproveToolCall}
          onRejectToolCall={onRejectToolCall}
        />
      ))}
      <div ref={bottomRef} />
    </div>
  )
}

function AiChatMessageRow({
  message,
  onApproveToolCall,
  onRejectToolCall,
}: Readonly<{
  message: AiChatMessage
  onApproveToolCall: (confirmationId: string) => void
  onRejectToolCall: (confirmationId: string) => void
}>) {
  if (message.role === 'tool') {
    return (
      <div className="flex items-start gap-2 pl-1 text-sm text-muted-foreground">
        <WrenchIcon className="mt-0.5 size-3.5 shrink-0" />
        <p className="whitespace-pre-wrap">{message.content}</p>
      </div>
    )
  }

  const isUser = message.role === 'user'

  return (
    <div className={cn('flex gap-2', isUser ? 'flex-row-reverse' : 'flex-row')}>
      <span
        className={cn(
          'mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full',
          isUser
            ? 'bg-primary text-primary-foreground'
            : 'bg-muted text-muted-foreground',
        )}
        aria-hidden="true"
      >
        {isUser ? (
          <UserIcon className="size-3.5" />
        ) : (
          <RobotIcon className="size-3.5" />
        )}
      </span>
      <div
        className={cn(
          'flex max-w-[85%] flex-col gap-2',
          isUser ? 'items-end' : 'items-start',
        )}
      >
        {message.content ? (
          <div
            className={cn(
              'rounded-lg px-3 py-2 text-sm whitespace-pre-wrap',
              isUser
                ? 'bg-primary text-primary-foreground'
                : 'bg-muted text-foreground',
            )}
          >
            {message.content}
          </div>
        ) : null}
        {(message.tool_calls ?? []).map((toolCall) => (
          <div key={toolCall.confirmation_id} className="w-full">
            <AiToolCallCard
              toolCall={toolCall}
              onApprove={() => {
                onApproveToolCall(toolCall.confirmation_id)
              }}
              onReject={() => {
                onRejectToolCall(toolCall.confirmation_id)
              }}
            />
          </div>
        ))}
      </div>
    </div>
  )
}
