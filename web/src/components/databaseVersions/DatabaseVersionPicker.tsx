import { useId, useRef, useState } from 'react'
import type { KeyboardEvent, FocusEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import {
  badgeFor,
  groupVersions,
  hasPrerelease,
  relativeAge,
  selectableVersions,
  sourceLink,
} from '@/lib/databaseVersions'
import type { VersionBadge, VersionGroupId } from '@/lib/databaseVersions'
import {
  useCheckDatabaseVersions,
  useDatabaseVersions,
} from '../../queries/databaseVersions'
import type { CatalogOrigin } from '../../types/databaseVersions'

const GROUP_KEYS = {
  recommended: 'group.recommended',
  stable: 'group.stable',
  lts: 'group.lts',
  prerelease: 'group.prerelease',
  eol: 'group.eol',
} as const satisfies Record<VersionGroupId, string>

const BADGE_KEYS = {
  stable: 'badge.stable',
  lts: 'badge.lts',
  rc: 'badge.rc',
  beta: 'badge.beta',
  eolSoon: 'badge.eolSoon',
  eol: 'badge.eol',
} as const satisfies Record<VersionBadge, string>

const BADGE_VARIANTS = {
  stable: 'success',
  lts: 'info',
  rc: 'accent',
  beta: 'accent',
  eolSoon: 'warning',
  eol: 'muted',
} as const satisfies Record<VersionBadge, string>

const ORIGIN_KEYS = {
  live: 'origin.live',
  published: 'origin.published',
  direct: 'origin.direct',
  embedded: 'origin.embedded',
} as const satisfies Record<CatalogOrigin, string>

export interface DatabaseVersionPickerProps {
  engine: string
  value: string
  onChange: (version: string) => void
  id?: string
  placeholder?: string
  className?: string
}

// A text field that also offers the engine's catalog versions. Free text
// stays valid: the field never forces a choice from the list.
export function DatabaseVersionPicker({
  engine,
  value,
  onChange,
  id,
  placeholder,
  className,
}: Readonly<DatabaseVersionPickerProps>) {
  const { t, i18n } = useTranslation('databaseVersions')
  const query = useDatabaseVersions(engine)
  const check = useCheckDatabaseVersions(engine)
  const [open, setOpen] = useState(false)
  const [showPre, setShowPre] = useState(false)
  const [active, setActive] = useState(-1)
  const rootRef = useRef<HTMLDivElement>(null)
  const listId = useId()

  const data = query.data
  const now = new Date()
  const groups = data ? groupVersions(data.versions, now, showPre) : []
  const selectable = selectableVersions(groups)
  const available = data !== undefined && data.versions.length > 0
  const expanded = open && available

  function choose(version: string) {
    onChange(version)
    setOpen(false)
    setActive(-1)
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (!available) return
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      setOpen(true)
      const step = e.key === 'ArrowDown' ? 1 : -1
      setActive((i) =>
        Math.min(Math.max(i + step, 0), Math.max(selectable.length - 1, 0)),
      )
    } else if (e.key === 'Enter' && expanded && active >= 0) {
      const picked = selectable[active]
      if (picked !== undefined) {
        e.preventDefault()
        choose(picked)
      }
    } else if (e.key === 'Escape' && expanded) {
      e.preventDefault()
      e.stopPropagation()
      setOpen(false)
    }
  }

  function onBlur(e: FocusEvent<HTMLDivElement>) {
    const next = e.relatedTarget
    if (!(next instanceof Node) || !rootRef.current?.contains(next)) {
      setOpen(false)
    }
  }

  function formatAge(iso: string | undefined): string {
    const age = relativeAge(iso, now)
    if (!age) return t('unknownAge')
    return new Intl.RelativeTimeFormat(i18n.language, {
      numeric: 'auto',
    }).format(age.value, age.unit)
  }

  const link = data ? sourceLink(data.source) : null
  const activeId =
    expanded && active >= 0 ? `${listId}-opt-${active}` : undefined
  const checkError = check.isError ? check.error.message : null

  return (
    <div ref={rootRef} className="relative" onBlur={onBlur}>
      <Input
        id={id}
        role={available ? 'combobox' : undefined}
        aria-expanded={available ? expanded : undefined}
        aria-controls={available ? listId : undefined}
        aria-autocomplete={available ? 'none' : undefined}
        aria-activedescendant={activeId}
        autoComplete="off"
        value={value}
        placeholder={placeholder}
        className={cn('font-mono', available && 'pr-8', className)}
        onChange={(e) => {
          onChange(e.target.value)
        }}
        onKeyDown={onKeyDown}
        onFocus={() => {
          if (available) setOpen(true)
        }}
      />
      {available ? (
        <button
          type="button"
          tabIndex={-1}
          aria-label={t('toggle')}
          aria-expanded={expanded}
          aria-controls={listId}
          className="absolute inset-y-0 right-0 flex w-8 items-center justify-center text-muted-foreground hover:text-foreground"
          onClick={() => {
            setOpen((o) => !o)
          }}
        >
          <ChevronDownGlassIcon className="size-5" aria-hidden="true" />
        </button>
      ) : null}
      {expanded && data ? (
        <div
          className="absolute z-50 mt-1 w-full rounded-lg border bg-popover text-popover-foreground shadow-md"
          onMouseDown={(e) => {
            e.preventDefault()
          }}
        >
          <div
            role="listbox"
            id={listId}
            aria-label={t('listLabel')}
            className="max-h-64 overflow-y-auto p-1"
          >
            {groups.map((group) => (
              <div
                key={group.id}
                role="group"
                aria-labelledby={`${listId}-${group.id}`}
              >
                <div
                  id={`${listId}-${group.id}`}
                  className="px-2 py-1 text-xs font-medium text-muted-foreground"
                >
                  {t(GROUP_KEYS[group.id])}
                </div>
                {group.versions.map((v) => {
                  const disabled = group.id === 'eol'
                  const idx = disabled ? -1 : selectable.indexOf(v.version)
                  const badge = badgeFor(v, now)
                  let note = ''
                  if (v.eol && (disabled || badge === 'eolSoon')) {
                    note = t(disabled ? 'eolOn' : 'eolSoonOn', { date: v.eol })
                  }
                  return (
                    <div
                      key={v.version}
                      id={disabled ? undefined : `${listId}-opt-${idx}`}
                      role="option"
                      aria-selected={v.version === value}
                      aria-disabled={disabled || undefined}
                      className={cn(
                        'flex items-center justify-between gap-2 rounded-md px-2 py-1.5 text-sm',
                        disabled
                          ? 'cursor-not-allowed opacity-60'
                          : 'cursor-pointer hover:bg-muted',
                        idx >= 0 && idx === active && 'bg-muted',
                        v.version === value && 'font-medium',
                      )}
                      onClick={() => {
                        if (!disabled) choose(v.version)
                      }}
                    >
                      <span className="font-mono">{v.version}</span>
                      <span className="flex items-center gap-2">
                        {note ? (
                          <span className="text-xs text-muted-foreground">
                            {note}
                          </span>
                        ) : null}
                        <Badge variant={BADGE_VARIANTS[badge]}>
                          {t(BADGE_KEYS[badge])}
                        </Badge>
                      </span>
                    </div>
                  )
                })}
              </div>
            ))}
          </div>
          <div className="space-y-1.5 border-t p-2 text-xs text-muted-foreground">
            {hasPrerelease(data.versions) ? (
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  className="accent-primary"
                  checked={showPre}
                  onChange={(e) => {
                    setShowPre(e.target.checked)
                    setActive(-1)
                  }}
                />
                {t('showPrerelease')}
              </label>
            ) : null}
            <p>
              {link ? (
                <>
                  {t('sourceLabel')}{' '}
                  <a
                    href={link.href}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="underline underline-offset-2 hover:text-foreground"
                  >
                    {link.name}
                  </a>
                  {'. '}
                </>
              ) : null}
              {t(ORIGIN_KEYS[data.origin], {
                when: formatAge(data.fetched_at ?? data.generated_at),
              })}
            </p>
            {data.freshness === 'aging' || data.freshness === 'stale' ? (
              <p
                className={
                  data.freshness === 'stale' ? 'text-tone-warning' : undefined
                }
              >
                {t(`freshness.${data.freshness}`)}
              </p>
            ) : null}
            <div className="flex items-center gap-2">
              <Button
                type="button"
                size="xs"
                variant="outline"
                disabled={check.isPending}
                onClick={() => {
                  check.mutate()
                }}
              >
                {check.isPending ? t('checking') : t('check')}
              </Button>
              <p role="status" aria-live="polite" className="min-w-0">
                {checkError
                  ? t('checkFailed', { error: checkError })
                  : check.data?.cooldown
                    ? t('checkedJustNow')
                    : check.isSuccess
                      ? t('checkDone')
                      : null}
              </p>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  )
}
