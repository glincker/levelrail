import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import {
  suggest,
  tokenAt,
  type Suggestion,
} from '../../lib/sqlConsole/complete'
import type { DbSchemaNode } from '../../types/databaseViewer'

// Plain textarea with a schema-aware suggestion list. Suggestions open
// while typing and with Ctrl+Space; Up/Down move, Enter or Tab accept,
// Escape closes. Run, save and new-tab shortcuts live in the parent.
export function SqlEditor({
  value,
  onChange,
  schemas,
  onFirstFocus,
}: {
  value: string
  onChange: (sql: string) => void
  schemas: DbSchemaNode[]
  onFirstFocus: () => void
}) {
  const { t } = useTranslation('databases')
  const ref = useRef<HTMLTextAreaElement>(null)
  const listId = useId()
  const focused = useRef(false)
  const pendingCaret = useRef<number | null>(null)
  const [caret, setCaret] = useState(0)
  const [closed, setClosed] = useState(true)
  const [forced, setForced] = useState(false)
  const [index, setIndex] = useState(0)

  const { start, token } = useMemo(() => tokenAt(value, caret), [value, caret])
  const items = useMemo(
    () => (closed ? [] : suggest(token, schemas, forced)),
    [closed, token, schemas, forced],
  )
  const open = items.length > 0
  const active = Math.min(index, Math.max(0, items.length - 1))

  useEffect(() => {
    if (pendingCaret.current !== null && ref.current) {
      ref.current.setSelectionRange(pendingCaret.current, pendingCaret.current)
      pendingCaret.current = null
    }
  }, [value])

  function accept(s: Suggestion) {
    const next = value.slice(0, start) + s.insert + value.slice(caret)
    pendingCaret.current = start + s.insert.length
    setCaret(start + s.insert.length)
    setClosed(true)
    setForced(false)
    onChange(next)
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.ctrlKey && e.key === ' ') {
      e.preventDefault()
      setForced(true)
      setClosed(false)
      setIndex(0)
      return
    }
    if (!open) {
      return
    }
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const step = e.key === 'ArrowDown' ? 1 : -1
      setIndex((active + step + items.length) % items.length)
    } else if (
      (e.key === 'Enter' || e.key === 'Tab') &&
      !e.metaKey &&
      !e.ctrlKey &&
      !e.shiftKey
    ) {
      e.preventDefault()
      const pick = items[active]
      if (pick) {
        accept(pick)
      }
    } else if (e.key === 'Escape') {
      e.preventDefault()
      setClosed(true)
    }
  }

  return (
    <div className="relative">
      <Textarea
        ref={ref}
        value={value}
        role="combobox"
        aria-expanded={open}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={open ? `${listId}-${String(active)}` : undefined}
        aria-label={t('viewer.console.editorLabel')}
        placeholder={t('viewer.console.placeholder')}
        spellCheck={false}
        className="min-h-32 font-mono text-sm"
        onFocus={() => {
          if (!focused.current) {
            focused.current = true
            onFirstFocus()
          }
        }}
        onBlur={() => {
          setClosed(true)
        }}
        onClick={(e) => {
          setCaret(e.currentTarget.selectionStart)
          setClosed(true)
        }}
        onChange={(e) => {
          setCaret(e.target.selectionStart)
          setClosed(false)
          setForced(false)
          setIndex(0)
          onChange(e.target.value)
        }}
        onKeyDown={onKeyDown}
      />
      {open ? (
        <ul
          id={listId}
          role="listbox"
          aria-label={t('viewer.console.editor.suggestions')}
          className="absolute bottom-2 left-2 z-20 max-h-56 w-80 max-w-[calc(100%-1rem)] overflow-auto rounded-md border border-border bg-popover p-1 text-sm shadow-md"
        >
          {items.map((s, i) => (
            <li
              key={`${s.kind}-${s.insert}-${s.detail ?? ''}`}
              id={`${listId}-${String(i)}`}
              role="option"
              aria-selected={i === active}
              className={cn(
                'flex cursor-pointer items-baseline justify-between gap-3 rounded px-2 py-1',
                i === active ? 'bg-accent text-accent-foreground' : '',
              )}
              onMouseDown={(e) => {
                e.preventDefault()
                accept(s)
              }}
            >
              <span
                className={cn(
                  'truncate font-mono text-xs',
                  s.kind === 'keyword' ? 'text-muted-foreground' : '',
                )}
              >
                {s.label}
              </span>
              <span className="truncate text-xs text-muted-foreground">
                {s.detail ?? ''}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
