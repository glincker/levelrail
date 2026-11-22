import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type {
  DeployTargetGroup,
  DeployTargetNode,
} from '../types/deployTargets'
import { formatBytes } from '../lib/format'
import { formatMoney } from '../lib/usageFormat'
import { BrandIcon, type BrandIconName } from './BrandIcon'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'

// Same sentinels PlacementFields exports; duplicated here to avoid a cycle.
const AUTO_VALUE = '__auto__'
const KNOWN_BRANDS = new Set<string>([
  'aws',
  'digitalocean',
  'hetzner',
  'linode',
  'azure',
  'oracle',
  'vultr',
])

interface DeployTargetPickerProps {
  id: string
  groups: DeployTargetGroup[]
  /** Node id; empty string is the local server. */
  value: string
  onValueChange: (value: string) => void
  withAuto?: boolean
  /** Shown when nothing matches the value instead of the local label. */
  placeholder?: string
  disabled?: boolean
}

// Grouped, searchable node chooser: one group per cloud account, then local and unmanaged.
export function DeployTargetPicker({
  id,
  groups,
  value,
  onValueChange,
  withAuto = false,
  placeholder,
  disabled = false,
}: DeployTargetPickerProps) {
  const { t } = useTranslation('common')
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const needle = query.trim().toLowerCase()

  const filtered = useMemo(
    () =>
      groups
        .map((g) => ({
          ...g,
          nodes: g.nodes.filter(
            (n) =>
              needle === '' ||
              n.name.toLowerCase().includes(needle) ||
              g.label.toLowerCase().includes(needle),
          ),
        }))
        .filter((g) => g.nodes.length > 0),
    [groups, needle],
  )

  const current = groups
    .flatMap((g) => g.nodes)
    .find((n) => n.id === value && value !== AUTO_VALUE)
  const triggerLabel =
    withAuto && value === AUTO_VALUE
      ? t('nodeSelect.auto')
      : (current?.name ?? placeholder ?? t('nodeSelect.picker.local'))

  // Cost and free capacity are shown only when the API reported them.
  const capacityHint = (n: DeployTargetNode): string => {
    const parts: string[] = []
    if (n.monthly_cost_estimate !== undefined) {
      parts.push(
        t('nodeSelect.picker.cost', {
          amount: formatMoney(
            n.monthly_cost_estimate,
            n.cost_currency ?? 'USD',
          ),
        }),
      )
    }
    const free = n.free_capacity
    if (free?.cpu_cores !== undefined) {
      parts.push(
        t('nodeSelect.picker.freeCpu', { value: free.cpu_cores.toFixed(1) }),
      )
    }
    if (free?.memory_bytes !== undefined) {
      parts.push(
        t('nodeSelect.picker.freeMemory', {
          value: formatBytes(free.memory_bytes),
        }),
      )
    }
    return parts.join(' · ')
  }

  const choose = (v: string) => {
    onValueChange(v)
    setOpen(false)
    setQuery('')
  }

  const groupLabel = (g: DeployTargetGroup) =>
    g.kind === 'local'
      ? t('nodeSelect.picker.local')
      : g.kind === 'unmanaged'
        ? t('nodeSelect.picker.unmanaged')
        : g.label

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            id={id}
            type="button"
            disabled={disabled}
            variant="outline"
            className="w-full justify-start"
          />
        }
      >
        {triggerLabel}
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80">
        <Input
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
          }}
          placeholder={t('nodeSelect.picker.search')}
          aria-label={t('nodeSelect.picker.search')}
        />
        <div className="max-h-72 space-y-2 overflow-y-auto" role="listbox">
          {withAuto && needle === '' ? (
            <PickerRow
              selected={value === AUTO_VALUE}
              onPick={() => {
                choose(AUTO_VALUE)
              }}
              name={t('nodeSelect.auto')}
            />
          ) : null}
          {filtered.length === 0 ? (
            <p className="px-2 py-1 text-muted-foreground">
              {t('nodeSelect.picker.empty')}
            </p>
          ) : null}
          {filtered.map((g) => (
            <div key={`${g.kind}-${g.account_id ?? ''}`} role="group">
              <p className="flex items-center gap-1.5 px-2 text-xs text-muted-foreground">
                {g.provider && KNOWN_BRANDS.has(g.provider) ? (
                  <BrandIcon
                    name={g.provider as BrandIconName}
                    className="size-3.5"
                  />
                ) : null}
                {groupLabel(g)}
              </p>
              {g.nodes.map((n) => (
                <PickerRow
                  key={n.id || 'local'}
                  selected={n.id === value}
                  onPick={() => {
                    choose(n.id)
                  }}
                  name={n.name}
                  hint={`${t(`nodeSelect.picker.health.${n.health}`)}${n.region ? ` · ${n.region}` : ''}`}
                  extra={capacityHint(n)}
                />
              ))}
            </div>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  )
}

function PickerRow({
  name,
  hint,
  extra,
  selected,
  onPick,
}: {
  name: string
  hint?: string
  extra?: string
  selected: boolean
  onPick: () => void
}) {
  return (
    <button
      type="button"
      role="option"
      aria-selected={selected}
      onClick={onPick}
      className="flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left hover:bg-accent aria-selected:bg-accent"
    >
      <span>{name}</span>
      {hint ? (
        <span className="text-right text-xs text-muted-foreground">
          {hint}
          {extra ? <span className="block">{extra}</span> : null}
        </span>
      ) : null}
    </button>
  )
}
