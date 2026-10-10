import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { ALL_ACTIONS } from '../../lib/iamDraft'
import { RiskBadge } from './RiskBadge'
import type { AbilityInfo } from '../../queries/iamBuilder'

const GROUP_ORDER = ['view', 'change', 'deploy', 'admin'] as const

/** ActionPicker chooses abilities from grouped, searchable, plain-language rows with a risk hint on each. */
export function ActionPicker({
  abilities,
  value,
  onChange,
  invalid,
}: {
  abilities: AbilityInfo[]
  value: string[]
  onChange: (next: string[]) => void
  invalid?: boolean
}) {
  const { t } = useTranslation('iam')
  const [query, setQuery] = useState('')
  const searchId = useId()
  const all = value.includes(ALL_ACTIONS)

  const grouped = useMemo(() => {
    const q = query.trim().toLowerCase()
    const matches = abilities.filter(
      (a) =>
        !q ||
        a.id.includes(q) ||
        a.title.toLowerCase().includes(q) ||
        a.description.toLowerCase().includes(q),
    )
    return GROUP_ORDER.map((g) => ({
      group: g,
      items: matches.filter((a) => a.group === g),
    })).filter((g) => g.items.length > 0)
  }, [abilities, query])

  const toggle = (id: string, checked: boolean) => {
    const base = value.filter((a) => a !== ALL_ACTIONS)
    onChange(checked ? [...base, id] : base.filter((a) => a !== id))
  }

  return (
    <div
      className={cn(
        'space-y-2 rounded-lg border p-2',
        invalid ? 'border-destructive' : 'border-border',
      )}
    >
      <div className="relative">
        <MagnifyingGlassIcon
          className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          id={searchId}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t('builder.actions.search')}
          aria-label={t('builder.actions.search')}
          className="pl-8"
        />
      </div>
      <label className="flex cursor-pointer items-start gap-2.5 rounded-md px-2 py-1.5 hover:bg-muted/50">
        <Checkbox
          checked={all}
          onCheckedChange={(c) => onChange(c ? [ALL_ACTIONS] : [])}
          aria-label={t('builder.actions.all')}
          className="mt-0.5"
        />
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium text-foreground">
            {t('builder.actions.all')}
          </span>
          <span className="block text-xs text-muted-foreground">
            {t('builder.actions.allHint')}
          </span>
        </span>
        <RiskBadge risk="root" />
      </label>
      <div className="max-h-64 space-y-2 overflow-y-auto">
        {grouped.length === 0 ? (
          <p className="px-2 py-3 text-sm text-muted-foreground">
            {t('builder.actions.noMatch')}
          </p>
        ) : null}
        {grouped.map(({ group, items }) => (
          <fieldset key={group} disabled={all} className="disabled:opacity-50">
            <legend className="px-2 text-xs font-medium text-muted-foreground uppercase">
              {t(`group.${group}`)}
            </legend>
            {items.map((a) => (
              <label
                key={a.id}
                className="flex cursor-pointer items-start gap-2.5 rounded-md px-2 py-1.5 hover:bg-muted/50"
              >
                <Checkbox
                  checked={all || value.includes(a.id)}
                  onCheckedChange={(c) => toggle(a.id, Boolean(c))}
                  aria-label={a.title}
                  className="mt-0.5"
                />
                <span className="min-w-0 flex-1">
                  <span className="block text-sm font-medium text-foreground">
                    {a.title}{' '}
                    <code className="font-mono text-xs font-normal text-muted-foreground">
                      {a.id}
                    </code>
                  </span>
                  <span className="block text-xs text-muted-foreground">
                    {a.description}
                  </span>
                </span>
                <RiskBadge risk={a.risk} />
              </label>
            ))}
          </fieldset>
        ))}
      </div>
    </div>
  )
}
