'use client'

import { useId, useMemo, useRef, useState, type ReactNode } from 'react'
import {
  CheckGlassIcon,
  ChevronDownGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

export interface SearchSelectOption {
  value: string
  label: string
  /** Right-aligned secondary text such as a clock or an English name. */
  trailing?: string
  /** Extra text matched by the filter but not shown. */
  keywords?: string
  /** Options sharing a group id render under one heading, in order. */
  group?: string
}

export interface SearchSelectProps {
  options: readonly SearchSelectOption[]
  value: string
  onValueChange: (value: string) => void
  /** Heading text per group id. */
  groupLabels?: Readonly<Record<string, string>>
  id?: string
  ariaLabel?: string
  searchPlaceholder: string
  emptyMessage: string
  onOpenChange?: (open: boolean) => void
  /** Leading icon in the trigger. */
  icon?: ReactNode
  className?: string
}

/** SearchSelect is a type-to-filter listbox in a popover with a check on the active option. */
export function SearchSelect({
  options,
  value,
  onValueChange,
  groupLabels,
  id,
  ariaLabel,
  searchPlaceholder,
  emptyMessage,
  onOpenChange,
  icon,
  className,
}: SearchSelectProps) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [highlighted, setHighlighted] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listId = useId()

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return options
    return options.filter((o) =>
      `${o.label} ${o.trailing ?? ''} ${o.keywords ?? ''}`
        .toLowerCase()
        .includes(q),
    )
  }, [options, search])

  const selected = options.find((o) => o.value === value)
  const optionId = (i: number) => `${listId}-opt-${i}`

  const changeOpen = (next: boolean) => {
    setOpen(next)
    onOpenChange?.(next)
    if (next) {
      setSearch('')
      setHighlighted(
        Math.max(
          options.findIndex((o) => o.value === value),
          0,
        ),
      )
      requestAnimationFrame(() => inputRef.current?.focus())
    }
  }

  const commit = (option: SearchSelectOption | undefined) => {
    if (!option) return
    onValueChange(option.value)
    changeOpen(false)
  }

  const move = (index: number) => {
    const next = Math.min(Math.max(index, 0), filtered.length - 1)
    setHighlighted(next)
    document.getElementById(optionId(next))?.scrollIntoView?.({
      block: 'nearest',
    })
  }

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      move(highlighted + 1)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      move(highlighted - 1)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      commit(filtered[highlighted])
    }
  }

  return (
    <Popover open={open} onOpenChange={changeOpen}>
      <PopoverTrigger
        id={id}
        role="combobox"
        aria-label={ariaLabel}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listId : undefined}
        className={cn(
          'flex h-9 w-full items-center gap-2 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none transition-colors hover:bg-muted/50 focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 motion-reduce:transition-none',
          className,
        )}
      >
        {icon}
        <span className="min-w-0 flex-1 truncate text-left">
          {selected?.label ?? value}
        </span>
        <ChevronDownGlassIcon
          className="size-5 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="w-(--anchor-width) min-w-72 p-1.5"
        onKeyDown={onKeyDown}
      >
        <div className="relative mb-1.5">
          <SearchGlassIcon
            className="pointer-events-none absolute top-1/2 left-2 size-5 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            ref={inputRef}
            value={search}
            role="searchbox"
            aria-label={searchPlaceholder}
            aria-controls={listId}
            aria-activedescendant={
              filtered.length > 0 ? optionId(highlighted) : undefined
            }
            placeholder={searchPlaceholder}
            className="h-8 pl-9 text-sm"
            onChange={(e) => {
              setSearch(e.target.value)
              setHighlighted(0)
            }}
          />
        </div>
        <div
          id={listId}
          role="listbox"
          aria-label={ariaLabel ?? searchPlaceholder}
          className="max-h-64 overflow-y-auto"
        >
          {filtered.length === 0 ? (
            <p className="px-2 py-3 text-center text-xs text-muted-foreground">
              {emptyMessage}
            </p>
          ) : (
            filtered.map((o, i) => (
              <div key={o.value}>
                {o.group && o.group !== filtered[i - 1]?.group ? (
                  <p
                    role="presentation"
                    className="px-2 pt-2 pb-1 text-xs font-medium text-muted-foreground"
                  >
                    {groupLabels?.[o.group] ?? o.group}
                  </p>
                ) : null}
                <div
                  id={optionId(i)}
                  role="option"
                  aria-selected={o.value === value}
                  tabIndex={-1}
                  onClick={() => commit(o)}
                  onMouseEnter={() => setHighlighted(i)}
                  className={cn(
                    'flex min-h-9 cursor-pointer items-center gap-2 rounded-md px-2 text-sm',
                    i === highlighted && 'bg-muted',
                  )}
                >
                  <span className="min-w-0 flex-1 truncate">{o.label}</span>
                  {o.trailing ? (
                    <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                      {o.trailing}
                    </span>
                  ) : null}
                  <CheckGlassIcon
                    className={cn(
                      'size-5 shrink-0 text-primary',
                      o.value !== value && 'invisible',
                    )}
                    aria-hidden="true"
                  />
                </div>
              </div>
            ))
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
