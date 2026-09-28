import { useState } from 'react'
import { BookmarkSimpleIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { TagFilter } from './TagFilter'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  EMPTY_FILTERS,
  filtersActive,
  loadSavedViews,
  storeSavedViews,
  type AppListFilters,
  type SavedAppView,
} from '../lib/appViews'

function toggle(list: string[], value: string): string[] {
  return list.includes(value)
    ? list.filter((v) => v !== value)
    : [...list, value]
}

function SavedViews({
  filters,
  onApply,
}: {
  filters: AppListFilters
  onApply: (f: AppListFilters) => void
}) {
  const [views, setViews] = useState<SavedAppView[]>(loadSavedViews)
  const [name, setName] = useState('')

  const persist = (next: SavedAppView[]) => {
    setViews(next)
    storeSavedViews(next)
  }
  const save = () => {
    const trimmed = name.trim()
    if (trimmed === '') {
      return
    }
    persist([
      ...views.filter((v) => v.name !== trimmed),
      { name: trimmed, filters },
    ])
    setName('')
  }

  return (
    <Popover>
      <PopoverTrigger
        render={<Button type="button" size="sm" variant="outline" />}
      >
        <BookmarkSimpleIcon />
        Views
      </PopoverTrigger>
      <PopoverContent align="end" className="w-64">
        <div className="flex flex-col gap-1">
          {views.length === 0 ? (
            <p className="px-1 text-xs text-muted-foreground">
              No saved views yet.
            </p>
          ) : null}
          {views.map((view) => (
            <div key={view.name} className="flex items-center gap-1">
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="flex-1 justify-start truncate"
                onClick={() => {
                  onApply(view.filters)
                }}
              >
                {view.name}
              </Button>
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                aria-label={`Delete view ${view.name}`}
                onClick={() => {
                  persist(views.filter((v) => v.name !== view.name))
                }}
              >
                <XIcon />
              </Button>
            </div>
          ))}
        </div>
        {filtersActive(filters) ? (
          <div className="mt-2 flex gap-1 border-t border-border pt-2">
            <Input
              value={name}
              placeholder="Save current filters as..."
              aria-label="Saved view name"
              onChange={(e) => {
                setName(e.target.value)
              }}
            />
            <Button type="button" size="sm" onClick={save}>
              Save
            </Button>
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  )
}

export function AppsFilterBar({
  filters,
  environments,
  onChange,
}: {
  filters: AppListFilters
  environments: string[]
  onChange: (next: AppListFilters) => void
}) {
  return (
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <Input
        value={filters.query}
        placeholder="Search apps"
        aria-label="Search apps"
        className="h-8 w-56"
        onChange={(e) => {
          onChange({ ...filters, query: e.target.value })
        }}
      />
      {environments.map((env) => {
        const on = filters.environments.includes(env)
        return (
          <Button
            key={env}
            type="button"
            size="sm"
            variant={on ? 'default' : 'outline'}
            aria-pressed={on}
            onClick={() => {
              onChange({
                ...filters,
                environments: toggle(filters.environments, env),
              })
            }}
          >
            {env}
          </Button>
        )
      })}
      <TagFilter
        selected={filters.tags}
        onChange={(tags) => {
          onChange({ ...filters, tags })
        }}
      />
      <SavedViews filters={filters} onApply={onChange} />
      {filtersActive(filters) ? (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => {
            onChange(EMPTY_FILTERS)
          }}
        >
          Clear
        </Button>
      ) : null}
    </div>
  )
}
