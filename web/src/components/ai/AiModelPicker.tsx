import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import type { AiModel } from '../../queries/aiProviders'
import { formatCount } from './aiProviderState'

export function AiModelPicker({
  id,
  value,
  onChange,
  models,
  loading,
  disabled,
  placeholder,
}: {
  id: string
  value: string
  onChange: (value: string) => void
  models: AiModel[]
  loading?: boolean
  disabled?: boolean
  placeholder?: string
}) {
  const { t } = useTranslation('settings')
  const listId = useId()
  const [open, setOpen] = useState(false)
  const [highlight, setHighlight] = useState(0)

  const filtered = useMemo(() => {
    const q = value.trim().toLowerCase()
    if (!q) return models
    return models.filter(
      (m) =>
        m.id.toLowerCase().includes(q) || m.label.toLowerCase().includes(q),
    )
  }, [models, value])

  function pick(model: AiModel | undefined) {
    if (!model) return
    onChange(model.id)
    setOpen(false)
  }

  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setOpen(true)
      setHighlight((i) => Math.min(i + 1, filtered.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setHighlight((i) => Math.max(i - 1, 0))
    } else if (e.key === 'Enter' && open && filtered[highlight]) {
      e.preventDefault()
      pick(filtered[highlight])
    } else if (e.key === 'Escape') {
      setOpen(false)
    }
  }

  const showList = open && !disabled && filtered.length > 0

  return (
    <div className="relative">
      <Input
        id={id}
        role="combobox"
        aria-expanded={showList}
        aria-controls={listId}
        aria-autocomplete="list"
        autoComplete="off"
        spellCheck={false}
        disabled={disabled}
        value={value}
        placeholder={loading ? t('aiAssistantPage.model.loading') : placeholder}
        onChange={(e) => {
          onChange(e.target.value)
          setOpen(true)
          setHighlight(0)
        }}
        onFocus={() => {
          setOpen(true)
        }}
        onBlur={() => {
          setOpen(false)
        }}
        onKeyDown={onKeyDown}
      />
      {showList ? (
        <ul
          id={listId}
          role="listbox"
          className="absolute z-10 mt-1 max-h-56 w-full overflow-y-auto rounded-lg border border-border bg-popover p-1 shadow-md"
        >
          {filtered.map((m, i) => (
            <li
              key={m.id}
              role="option"
              aria-selected={m.id === value}
              onMouseDown={(e) => {
                e.preventDefault()
                pick(m)
              }}
              className={cn(
                'flex cursor-pointer items-center justify-between gap-2 rounded-md px-2 py-1.5 text-sm',
                i === highlight && 'bg-muted',
              )}
            >
              <span className="min-w-0 truncate">{m.label || m.id}</span>
              {m.context_window ? (
                <span className="shrink-0 text-xs text-muted-foreground">
                  {t('aiAssistantPage.model.context', {
                    size: formatCount(m.context_window),
                  })}
                </span>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
