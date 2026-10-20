'use client'

import { useMemo, useRef, useState } from 'react'
import {
  CaretDownIcon,
  MagnifyingGlassIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

export interface ComboboxOption {
  value: string
  label: string
  description?: string
  disabled?: boolean
}

// Searchable select built on Popover + Input (no cmdk/Command primitive
// here). Not virtualized: tanstack-virtual needs layout jsdom can't give
// it, and search already keeps the visible set small.

export function Combobox({
  options,
  value,
  onValueChange,
  placeholder = 'Select...',
  searchPlaceholder = 'Search...',
  emptyMessage = 'No results.',
  disabled,
  isLoading,
  id,
  triggerClassName,
}: Readonly<{
  options: ComboboxOption[]
  value: string
  onValueChange: (value: string) => void
  placeholder?: string
  searchPlaceholder?: string
  emptyMessage?: string
  disabled?: boolean
  isLoading?: boolean
  id?: string
  triggerClassName?: string
}>) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [highlighted, setHighlighted] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return options
    return options.filter((option) =>
      option.label.toLowerCase().includes(query),
    )
  }, [options, search])

  const selected = options.find((option) => option.value === value)
  let triggerLabel = placeholder
  if (isLoading) {
    triggerLabel = 'Loading...'
  } else if (selected) {
    triggerLabel = selected.label
  }

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setSearch('')
      setHighlighted(0)
      requestAnimationFrame(() => inputRef.current?.focus())
    }
  }

  function commit(option: ComboboxOption | undefined) {
    if (!option || option.disabled) return
    onValueChange(option.value)
    setOpen(false)
  }

  function handleKeyDown(event: React.KeyboardEvent) {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setHighlighted((index) => Math.min(index + 1, filtered.length - 1))
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      setHighlighted((index) => Math.max(index - 1, 0))
    } else if (event.key === 'Enter') {
      event.preventDefault()
      commit(filtered[highlighted])
    } else if (event.key === 'Escape') {
      setOpen(false)
    }
  }

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger
        id={id}
        disabled={disabled}
        role="combobox"
        aria-expanded={open}
        className={cn(
          'flex h-8 w-full items-center justify-between gap-2 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50',
          triggerClassName,
        )}
      >
        <span
          className={cn(
            'truncate text-left',
            !selected && 'text-muted-foreground',
          )}
        >
          {triggerLabel}
        </span>
        <CaretDownIcon
          className="size-3.5 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-64 p-1.5"
        onKeyDown={handleKeyDown}
      >
        <div className="relative mb-1.5">
          <MagnifyingGlassIcon
            className="pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            ref={inputRef}
            value={search}
            onChange={(event) => {
              setSearch(event.target.value)
              setHighlighted(0)
            }}
            placeholder={searchPlaceholder}
            aria-label={searchPlaceholder}
            className="h-7 pl-7 text-sm"
          />
        </div>
        <div ref={listRef} role="listbox" className="max-h-56 overflow-y-auto">
          {filtered.length === 0 ? (
            <p className="px-2 py-3 text-center text-xs text-muted-foreground">
              {emptyMessage}
            </p>
          ) : (
            filtered.map((option, index) => (
              <button
                key={option.value}
                type="button"
                role="option"
                aria-selected={option.value === value}
                disabled={option.disabled}
                onClick={() => commit(option)}
                onMouseEnter={() => setHighlighted(index)}
                className={cn(
                  'flex w-full flex-col items-start justify-center gap-0.5 rounded-md px-2 py-1 text-left text-sm transition-colors',
                  index === highlighted ? 'bg-muted' : 'hover:bg-muted',
                  option.value === value && 'font-medium text-primary',
                  option.disabled && 'pointer-events-none opacity-50',
                )}
              >
                <span className="w-full truncate">{option.label}</span>
                {option.description ? (
                  <span className="w-full truncate text-xs text-muted-foreground">
                    {option.description}
                  </span>
                ) : null}
              </button>
            ))
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
