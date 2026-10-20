import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { PlusIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import type { ComboboxOption } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { matchResources } from '../../queries/iamBuilder'
import type { IamResources, ResourceMatch } from '../../queries/iamBuilder'
import {
  selectionKey,
  selectionToResources,
  type ResourceSelection,
} from '../../lib/iamDraft'

type Kind = ResourceSelection['kind']

const KINDS: Kind[] = [
  'all',
  'project',
  'environment',
  'environment_kind',
  'app',
  'database',
  'pattern',
]
const ENVIRONMENT_KINDS = [
  'dev',
  'test',
  'uat',
  'production',
  'preview',
  'custom',
]

function optionsFor(kind: Kind, res: IamResources): ComboboxOption[] {
  switch (kind) {
    case 'project':
      return res.projects.map((p) => ({ value: p.id, label: p.name }))
    case 'environment':
      return res.environments.map((e) => ({
        value: e.id,
        label: `${e.name} (${e.kind})`,
      }))
    case 'environment_kind':
      return ENVIRONMENT_KINDS.map((k) => ({ value: k, label: k }))
    case 'app':
      return res.apps.map((a) => ({ value: a.name, label: a.name }))
    case 'database':
      return res.databases.map((d) => ({ value: d.name, label: d.name }))
    default:
      return []
  }
}

function selectionFor(
  kind: Kind,
  value: string,
  res: IamResources,
): ResourceSelection | null {
  switch (kind) {
    case 'all':
      return { kind: 'all' }
    case 'pattern':
      return value.trim() ? { kind: 'pattern', value: value.trim() } : null
    case 'project': {
      const p = res.projects.find((x) => x.id === value)
      return p
        ? { kind: 'project', id: p.id, environmentIds: p.environment_ids }
        : null
    }
    case 'environment':
      return value ? { kind: 'environment', id: value } : null
    case 'environment_kind':
      return value ? { kind: 'environment_kind', value } : null
    case 'app':
      return value ? { kind: 'app', name: value } : null
    case 'database':
      return value ? { kind: 'database', name: value } : null
  }
}

function labelFor(sel: ResourceSelection, res: IamResources): string {
  switch (sel.kind) {
    case 'all':
      return '*'
    case 'project':
      return res.projects.find((p) => p.id === sel.id)?.name ?? sel.id
    case 'environment': {
      const e = res.environments.find((x) => x.id === sel.id)
      return e ? `${e.name} (${e.kind})` : sel.id
    }
    case 'environment_kind':
      return `environment-kind:${sel.value}`
    case 'app':
      return `app:${sel.name}`
    case 'database':
      return `database:${sel.name}`
    case 'pattern':
      return sel.value
  }
}

/** ResourcePicker chooses what a rule applies to, by picker, and shows how many apps and databases each choice matches today. */
export function ResourcePicker({
  resources,
  value,
  onChange,
  invalid,
  idPrefix,
}: {
  resources: IamResources
  value: ResourceSelection[]
  onChange: (next: ResourceSelection[]) => void
  invalid?: boolean
  idPrefix: string
}) {
  const { t } = useTranslation('iam')
  const [kind, setKind] = useState<Kind>('app')
  const [picked, setPicked] = useState('')

  const strings = useMemo(() => value.flatMap(selectionToResources), [value])
  const match = useQuery({
    queryKey: ['iam-match', strings],
    queryFn: () => matchResources(strings),
    enabled: strings.length > 0,
  })

  const countsBySelection = useMemo(() => {
    const out = new Map<string, ResourceMatch[]>()
    let i = 0
    for (const sel of value) {
      const n = selectionToResources(sel).length
      out.set(selectionKey(sel), (match.data ?? []).slice(i, i + n))
      i += n
    }
    return out
  }, [value, match.data])

  const options = optionsFor(kind, resources)
  const add = () => {
    const sel = selectionFor(kind, picked, resources)
    if (!sel) return
    if (value.some((v) => selectionKey(v) === selectionKey(sel))) return
    onChange([...value, sel])
    setPicked('')
  }

  return (
    <div className="space-y-2">
      <ul
        className={cn(
          'space-y-1.5 rounded-lg border p-2',
          invalid ? 'border-destructive' : 'border-border',
        )}
        aria-label={t('builder.resources.label')}
      >
        {value.length === 0 ? (
          <li className="px-1 py-1 text-sm text-muted-foreground">
            {t('builder.resources.required')}
          </li>
        ) : null}
        {value.map((sel) => {
          const rows = countsBySelection.get(selectionKey(sel)) ?? []
          const apps = rows.reduce((n, r) => n + r.apps, 0)
          const dbs = rows.reduce((n, r) => n + r.databases, 0)
          const label = labelFor(sel, resources)
          return (
            <li
              key={selectionKey(sel)}
              className="flex items-center gap-2 rounded-md bg-muted/40 px-2 py-1.5"
            >
              <div className="min-w-0 flex-1">
                <code className="text-sm break-all text-foreground">
                  {label}
                </code>
                <p className="text-xs text-muted-foreground">
                  {match.isPending && strings.length > 0
                    ? t('builder.resources.matchChecking')
                    : apps + dbs === 0
                      ? t('builder.resources.matchNone')
                      : `${t('builder.resources.matchApps', { count: apps })}, ${t('builder.resources.matchDatabases', { count: dbs })}`}
                  {sel.kind === 'project'
                    ? `. ${t('builder.resources.projectNote', { count: sel.environmentIds.length })}`
                    : ''}
                </p>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={t('builder.resources.remove', { resource: label })}
                onClick={() =>
                  onChange(
                    value.filter((v) => selectionKey(v) !== selectionKey(sel)),
                  )
                }
              >
                <XIcon />
              </Button>
            </li>
          )
        })}
      </ul>
      <div className="flex flex-wrap items-center gap-2">
        <Select
          value={kind}
          onValueChange={(v) => {
            setKind(v as Kind)
            setPicked('')
          }}
        >
          <SelectTrigger
            id={`${idPrefix}-kind`}
            aria-label={t('builder.resources.kind')}
            className="w-48"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {KINDS.map((k) => (
              <SelectItem key={k} value={k}>
                {t(`builder.resources.kinds.${k}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {kind === 'pattern' ? (
          <Input
            value={picked}
            onChange={(e) => setPicked(e.target.value)}
            placeholder={t('builder.resources.patternPlaceholder')}
            aria-label={t('builder.resources.kinds.pattern')}
            className="w-56"
          />
        ) : kind !== 'all' ? (
          <Combobox
            options={options}
            value={picked}
            onValueChange={setPicked}
            placeholder={t('builder.resources.value')}
            searchPlaceholder={t('builder.resources.search')}
            emptyMessage={t('builder.resources.noOptions')}
            triggerClassName="w-56"
          />
        ) : null}
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={add}
          disabled={kind !== 'all' && !picked.trim()}
        >
          <PlusIcon />
          {t('builder.resources.add')}
        </Button>
      </div>
      {kind === 'pattern' ? (
        <p className="text-xs text-muted-foreground">
          {t('builder.resources.patternHint')}
        </p>
      ) : null}
    </div>
  )
}
