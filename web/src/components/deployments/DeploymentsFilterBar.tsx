import { useEffect, useState } from 'react'
import { MagnifyingGlassIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import {
  activeFilterCount,
  type DeploymentFilters,
  type Facets,
  type FilterKey,
} from '../../lib/deploymentFilters'
import { AddFilterMenu } from './AddFilterMenu'
import { pillsFor } from '../../lib/deploymentFilterLabels'

const SEARCH_DEBOUNCE_MS = 300

export interface DeploymentsFilterBarProps {
  filters: DeploymentFilters
  facets: Facets
  addOpen: boolean
  onAddOpenChange: (open: boolean) => void
  onSearch: (q: string) => void
  onToggle: (key: FilterKey, value: string) => void
  onRemove: (key: FilterKey, value: string) => void
  onClear: () => void
}

export function DeploymentsFilterBar({
  filters,
  facets,
  addOpen,
  onAddOpenChange,
  onSearch,
  onToggle,
  onRemove,
  onClear,
}: DeploymentsFilterBarProps) {
  const [text, setText] = useState(filters.q)
  const debounced = useDebouncedValue(text, SEARCH_DEBOUNCE_MS)
  const [urlQ, setUrlQ] = useState(filters.q)
  if (filters.q !== urlQ) {
    setUrlQ(filters.q)
    setText(filters.q)
  }

  useEffect(() => {
    if (debounced !== filters.q) onSearch(debounced)
    // Only a settled edit should push to the URL; a URL-driven change is handled above.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debounced])

  const pills = pillsFor(filters)
  const count = activeFilterCount(filters)

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative w-full min-w-48 sm:w-72">
          <MagnifyingGlassIcon
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            data-shortcut-search
            value={text}
            placeholder="Search commit, sha or app"
            aria-label="Search deployments"
            className="pl-8"
            onChange={(e) => {
              setText(e.target.value)
            }}
          />
        </div>
        <AddFilterMenu
          filters={filters}
          facets={facets}
          open={addOpen}
          onOpenChange={onAddOpenChange}
          onToggle={onToggle}
        />
      </div>
      {(pills.length > 0 || filters.q) && (
        <ul
          aria-label="Active filters"
          className="flex flex-wrap items-center gap-1.5"
        >
          {pills.map((p) => (
            <li key={`${p.key}:${p.value}`}>
              <span className="inline-flex h-6 items-center gap-1 rounded-full border border-border bg-muted/50 pr-0.5 pl-2.5 text-xs">
                {p.text}
                <button
                  type="button"
                  aria-label={`Remove filter ${p.text}`}
                  onClick={() => {
                    onRemove(p.key, p.value)
                  }}
                  className="flex size-5 items-center justify-center rounded-full text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
                >
                  <XIcon className="size-3" aria-hidden="true" />
                </button>
              </span>
            </li>
          ))}
          {count > 0 && (
            <li>
              <Button variant="ghost" size="xs" onClick={onClear}>
                Clear all
              </Button>
            </li>
          )}
        </ul>
      )}
    </div>
  )
}
