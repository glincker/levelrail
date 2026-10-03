import { useEffect, useMemo, useRef } from 'react'
import { RobotIcon, UserIcon, WrenchIcon } from '@phosphor-icons/react/dist/ssr'
import type { AiChatMessage } from '../types/aiAssistant'
import { AiToolCallCard } from './AiToolCallCard'
import { renderChatMarkdown } from '../lib/renderChatMarkdown'
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
  const isUser = message.role === 'user'
  // Only the assistant's own prose is rendered as markdown: the user's
  // typed message stays plain text, matching how every other chat UI
  // treats what you yourself typed versus what came back. Computed
  // unconditionally (even for a 'tool' row, which never reads it) so
  // this hook's call order never varies across renders.
  const assistantHtml = useMemo(
    () =>
      isUser || message.role === 'tool'
        ? ''
        : renderChatMarkdown(message.content),
    [isUser, message.role, message.content],
  )

  if (message.role === 'tool') {
    return (
      <div className="flex items-start gap-2 pl-1 text-sm text-muted-foreground">
        <WrenchIcon className="mt-0.5 size-3.5 shrink-0" />
        <p className="whitespace-pre-wrap">{message.content}</p>
      </div>
    )
  }

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
          isUser ? (
            <div className="rounded-lg bg-primary px-3 py-2 text-sm whitespace-pre-wrap text-primary-foreground">
              {message.content}
            </div>
          ) : (
            <div
              className="max-w-full rounded-lg bg-muted px-3 py-2 text-sm text-foreground [&_a]:break-all [&_code]:rounded [&_code]:bg-background [&_code]:px-1 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-xs [&_li]:my-0.5 [&_ol]:my-1.5 [&_ol]:list-decimal [&_ol]:pl-5 [&_p]:my-1.5 [&_p:first-child]:mt-0 [&_p:last-child]:mb-0 [&_pre]:my-1.5 [&_pre]:overflow-x-auto [&_pre]:rounded-md [&_pre]:bg-background [&_pre]:p-2 [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_ul]:my-1.5 [&_ul]:list-disc [&_ul]:pl-5"
              // Safe: renderChatMarkdown (lib/renderChatMarkdown.ts) drops
              // raw HTML and images outright and only emits http(s)/
              // mailto/relative links, specifically so untrusted text a
              // tool call pulled in can never become markup here.
              dangerouslySetInnerHTML={{ __html: assistantHtml }}
            />
          )
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
