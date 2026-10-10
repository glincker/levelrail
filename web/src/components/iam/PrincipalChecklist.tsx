import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { principalKey } from '../../lib/iamDraft'
import type { IamPrincipal } from '../../queries/iamBuilder'
import { VirtualRows } from './VirtualRows'

/** PrincipalChecklist picks users and tokens, with a search box and a select-all-tokens shortcut for bulk attach. */
export function PrincipalChecklist({
  principals,
  selected,
  onChange,
  title,
  description,
}: {
  principals: IamPrincipal[]
  selected: string[]
  onChange: (next: string[]) => void
  title?: string
  description?: string
}) {
  const { t } = useTranslation('iam')
  const [query, setQuery] = useState('')
  const shown = useMemo(() => {
    const q = query.trim().toLowerCase()
    return principals.filter(
      (p) =>
        !q ||
        p.name.toLowerCase().includes(q) ||
        p.principal_id.toLowerCase().includes(q) ||
        (p.detail ?? '').toLowerCase().includes(q),
    )
  }, [principals, query])
  const shownTokens = shown.filter((p) => p.principal_type === 'token')

  const toggle = (key: string, on: boolean) =>
    onChange(on ? [...selected, key] : selected.filter((k) => k !== key))

  return (
    <section className="space-y-2">
      <div>
        <h3 className="text-sm font-medium text-foreground">
          {title ?? t('builder.attach.title')}
        </h3>
        <p className="text-xs text-muted-foreground">
          {description ?? t('builder.attach.description')}
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t('access.search')}
          aria-label={t('access.search')}
          className="max-w-xs"
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={shownTokens.length === 0}
          onClick={() =>
            onChange([
              ...new Set([...selected, ...shownTokens.map(principalKey)]),
            ])
          }
        >
          {t('access.selectAllTokens')}
        </Button>
        {selected.length > 0 ? (
          <>
            <span className="text-xs text-muted-foreground">
              {t('access.selectedCount', { count: selected.length })}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => onChange([])}
            >
              {t('access.clearSelection')}
            </Button>
          </>
        ) : null}
      </div>
      <VirtualRows
        items={shown}
        label={title ?? t('builder.attach.title')}
        rowKey={principalKey}
        maxHeightClass="max-h-56"
        renderRow={(p) => {
          const key = principalKey(p)
          return (
            <label className="flex h-14 cursor-pointer items-center gap-3 px-3 hover:bg-muted/50">
              <Checkbox
                checked={selected.includes(key)}
                onCheckedChange={(c) => toggle(key, Boolean(c))}
                aria-label={p.name}
              />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm text-foreground">
                  {p.name}
                </span>
                <span className="block truncate text-xs text-muted-foreground">
                  {p.detail || p.principal_id}
                </span>
              </span>
              <Badge variant="outline">{t(`access.${p.principal_type}`)}</Badge>
            </label>
          )
        }}
      />
      {shown.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('access.empty.description')}
        </p>
      ) : null}
    </section>
  )
}
