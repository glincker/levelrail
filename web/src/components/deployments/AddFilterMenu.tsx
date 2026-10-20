import { useState } from 'react'
import {
  CaretLeftIcon,
  CheckIcon,
  FunnelSimpleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Kbd } from '@/components/kit'
import { cn } from '@/lib/utils'
import type {
  DeploymentFilters,
  Facets,
  FilterKey,
} from '../../lib/deploymentFilters'
import {
  DEPLOYMENT_STATUSES,
  DEPLOYMENT_TRIGGERS,
} from '../../types/deployment'
import { FILTER_LABEL, optionLabel } from '../../lib/deploymentFilterLabels'

const FILTER_ORDER: FilterKey[] = [
  'author',
  'environment',
  'status',
  'app',
  'branch',
  'trigger',
]

const MULTI: FilterKey[] = ['status', 'trigger']

function optionsFor(key: FilterKey, facets: Facets): string[] {
  switch (key) {
    case 'status':
      return [...DEPLOYMENT_STATUSES]
    case 'trigger':
      return [...DEPLOYMENT_TRIGGERS]
    case 'environment': {
      const base = ['production', 'preview']
      return [
        ...base,
        ...facets.environment.filter((e) => !base.includes(e.toLowerCase())),
      ]
    }
    default:
      return facets[key]
  }
}

function isSelected(f: DeploymentFilters, key: FilterKey, v: string): boolean {
  if (key === 'status' || key === 'trigger') return f[key].includes(v)
  return f[key] === v
}

function OptionButton({
  selected,
  label,
  onClick,
}: {
  selected: boolean
  label: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      role="menuitemcheckbox"
      aria-checked={selected}
      onClick={onClick}
      className={cn(
        'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm outline-none hover:bg-muted focus-visible:bg-muted',
        selected && 'font-medium',
      )}
    >
      <CheckIcon
        className={cn('size-3.5', !selected && 'invisible')}
        aria-hidden="true"
      />
      <span className="truncate">{label}</span>
    </button>
  )
}

export interface AddFilterMenuProps {
  filters: DeploymentFilters
  facets: Facets
  open: boolean
  onOpenChange: (open: boolean) => void
  onToggle: (key: FilterKey, value: string) => void
}

export function AddFilterMenu({
  filters,
  facets,
  open,
  onOpenChange,
  onToggle,
}: AddFilterMenuProps) {
  const [key, setKey] = useState<FilterKey | null>(null)
  const [typed, setTyped] = useState('')

  const changeOpen = (next: boolean) => {
    if (!next) {
      setKey(null)
      setTyped('')
    }
    onOpenChange(next)
  }

  const commit = (k: FilterKey, v: string) => {
    onToggle(k, v)
    if (!MULTI.includes(k)) changeOpen(false)
  }

  const options = key ? optionsFor(key, facets) : []
  const shown = options.filter((o) =>
    optionLabel(key ?? 'app', o)
      .toLowerCase()
      .includes(typed.toLowerCase()),
  )
  const canType = key !== null && !MULTI.includes(key)

  return (
    <Popover open={open} onOpenChange={changeOpen}>
      <PopoverTrigger
        render={
          <Button variant="outline" size="sm" aria-label="Add filter">
            <FunnelSimpleIcon aria-hidden="true" />
            Add Filter
            <span className="hidden sm:inline">
              <Kbd keys={['f']} />
            </span>
          </Button>
        }
      />
      <PopoverContent align="start" className="w-64 gap-1 p-1.5">
        {key === null ? (
          <div role="menu" aria-label="Filter by" className="flex flex-col">
            {FILTER_ORDER.map((k) => (
              <button
                key={k}
                type="button"
                role="menuitem"
                onClick={() => {
                  setKey(k)
                }}
                className="rounded-md px-2 py-1.5 text-left text-sm outline-none hover:bg-muted focus-visible:bg-muted"
              >
                {FILTER_LABEL[k]}
              </button>
            ))}
          </div>
        ) : (
          <div className="flex flex-col gap-1">
            <button
              type="button"
              onClick={() => {
                setKey(null)
                setTyped('')
              }}
              className="flex items-center gap-1 rounded-md px-1.5 py-1 text-xs font-medium text-muted-foreground outline-none hover:bg-muted focus-visible:bg-muted"
            >
              <CaretLeftIcon className="size-3" aria-hidden="true" />
              {FILTER_LABEL[key]}
            </button>
            {canType && (
              <Input
                autoFocus
                value={typed}
                placeholder={`Find or type ${FILTER_LABEL[key].toLowerCase()}`}
                aria-label={`${FILTER_LABEL[key]} value`}
                onChange={(e) => {
                  setTyped(e.target.value)
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && typed.trim()) {
                    e.preventDefault()
                    commit(key, typed.trim())
                  }
                }}
              />
            )}
            <div
              role="menu"
              aria-label={FILTER_LABEL[key]}
              className="flex max-h-64 flex-col overflow-y-auto"
            >
              {shown.length === 0 && (
                <p className="px-2 py-1.5 text-xs text-muted-foreground">
                  {canType && typed
                    ? 'Press Enter to use this value.'
                    : 'Nothing loaded yet.'}
                </p>
              )}
              {shown.map((o) => (
                <OptionButton
                  key={o}
                  selected={isSelected(filters, key, o)}
                  label={optionLabel(key, o)}
                  onClick={() => {
                    commit(key, o)
                  }}
                />
              ))}
            </div>
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}
