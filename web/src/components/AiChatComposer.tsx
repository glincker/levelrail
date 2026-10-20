import { useState, type FormEvent, type KeyboardEvent } from 'react'
import { PaperPlaneRightIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'

export function AiChatComposer({
  onSend,
  disabled,
}: Readonly<{
  onSend: (content: string) => void
  disabled: boolean
}>) {
  const [value, setValue] = useState('')

  function submit() {
    const trimmed = value.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setValue('')
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    submit()
  }

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
    }
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="flex items-end gap-2 border-t border-border pt-3"
    >
      <Textarea
        value={value}
        onChange={(e) => {
          setValue(e.target.value)
        }}
        onKeyDown={handleKeyDown}
        placeholder="Ask about logs, metrics, deploy history..."
        disabled={disabled}
        className="min-h-10 flex-1 resize-none"
        aria-label="Message"
      />
      <Button type="submit" size="icon" disabled={disabled || !value.trim()}>
        <PaperPlaneRightIcon />
        <span className="sr-only">Send</span>
      </Button>
    </form>
  )
}
