import {
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useTranslation } from 'react-i18next'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { caretPosition } from '../../lib/sqlConsole/caret'
import { editorKeyAction } from '../../lib/sqlConsole/keys'
import {
  previousWord,
  suggest,
  tokenAt,
  type Suggestion,
} from '../../lib/sqlConsole/complete'
import type { DbSchemaNode } from '../../types/databaseViewer'

const LIST_ROW_PX = 28
const LIST_PAD_PX = 8
const LIST_MAX_PX = 224
const LIST_WIDTH_PX = 320

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
    () =>
      closed ? [] : suggest(token, schemas, forced, previousWord(value, start)),
    [closed, token, schemas, forced, value, start],
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
    const action = editorKeyAction({
      key: e.key,
      ctrlKey: e.ctrlKey,
      metaKey: e.metaKey,
      shiftKey: e.shiftKey,
      altKey: e.altKey,
      popupOpen: open,
    })
    if (action === 'run') {
      // The console handles the shortcut; just make sure the list goes away.
      setClosed(true)
    } else if (action === 'open') {
      e.preventDefault()
      setForced(true)
      setClosed(false)
      setIndex(0)
    } else if (action === 'next' || action === 'prev') {
      e.preventDefault()
      setIndex(
        (active + (action === 'next' ? 1 : -1) + items.length) % items.length,
      )
    } else if (action === 'accept') {
      e.preventDefault()
      const pick = items[active]
      if (pick) {
        accept(pick)
      }
    } else if (action === 'close') {
      e.preventDefault()
      setClosed(true)
    }
  }

  // Anchor at the caret; flip above the line when the list would overflow.
  const [place, setPlace] = useState({ top: 0, left: 0 })
  const count = items.length
  useLayoutEffect(() => {
    const el = ref.current
    if (count === 0 || !el) return
    const c = caretPosition(el, caret)
    const listH = Math.min(count * LIST_ROW_PX + LIST_PAD_PX, LIST_MAX_PX)
    const below = c.top + c.height + listH <= el.clientHeight
    setPlace({
      top: below ? c.top + c.height : Math.max(0, c.top - listH),
      left: Math.max(0, Math.min(c.left, el.clientWidth - LIST_WIDTH_PX)),
    })
  }, [count, caret, value])

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
          style={{ top: place.top, left: place.left }}
          className="absolute z-20 max-h-56 w-80 max-w-full overflow-auto rounded-md border border-border bg-popover p-1 text-sm shadow-md"
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
