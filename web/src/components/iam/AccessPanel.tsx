import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  KeyIcon,
  LinkBreakIcon,
  PlusIcon,
  UserIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { policyListQueryOptions } from '../../queries/iamPolicies'
import { iamPrincipalsQueryOptions } from '../../queries/iamBuilder'
import type { IamPrincipal } from '../../queries/iamBuilder'
import { AttachDialog } from './AttachDialog'
import { EffectivePermissions } from './EffectivePermissions'
import { principalKey } from '../../lib/iamDraft'
import { VirtualRows } from './VirtualRows'

type Filter = 'all' | 'user' | 'token'
const FILTERS: Filter[] = ['all', 'user', 'token']

function PrincipalRow({
  p,
  active,
  checked,
  onSelect,
  onCheck,
}: {
  p: IamPrincipal
  active: boolean
  checked: boolean
  onSelect: () => void
  onCheck: (on: boolean) => void
}) {
  const { t } = useTranslation('iam')
  const Icon = p.principal_type === 'user' ? UserIcon : KeyIcon
  return (
    <div
      className={cn(
        'flex h-14 items-center gap-3 px-3',
        active && 'bg-muted/60',
      )}
    >
      {p.principal_type === 'token' ? (
        <Checkbox
          checked={checked}
          onCheckedChange={(c) => onCheck(Boolean(c))}
          aria-label={p.name}
        />
      ) : (
        <span className="size-4" aria-hidden="true" />
      )}
      <button
        type="button"
        aria-pressed={active}
        onClick={onSelect}
        className="flex min-w-0 flex-1 items-center gap-3 text-left outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <Icon className="size-4 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm text-foreground">
            {p.name}
          </span>
          <span className="block truncate text-xs text-muted-foreground">
            {p.detail || p.principal_id}
          </span>
        </span>
        {!p.active ? (
          <Badge variant="muted">{t('access.inactive')}</Badge>
        ) : null}
        <Badge variant="outline">
          {t('access.policiesCount', { count: p.policy_ids.length })}
        </Badge>
      </button>
    </div>
  )
}

/** AccessPanel starts from a user or token: what it can do, which policies shape that, and attach or detach with a preview. */
export function AccessPanel({ initialKey }: { initialKey?: string }) {
  const { t } = useTranslation('iam')
  const principals = useQuery(iamPrincipalsQueryOptions())
  const policies = useQuery(policyListQueryOptions())
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const [active, setActive] = useState<string>(initialKey ?? '')
  const [checked, setChecked] = useState<string[]>([])
  const [dialog, setDialog] = useState<{
    mode: 'attach' | 'detach'
    policyId?: string
    keys: string[]
  } | null>(null)

  const all = useMemo(() => principals.data ?? [], [principals.data])
  const shown = useMemo(() => {
    const q = query.trim().toLowerCase()
    return all.filter(
      (p) =>
        (filter === 'all' || p.principal_type === filter) &&
        (!q ||
          p.name.toLowerCase().includes(q) ||
          p.principal_id.toLowerCase().includes(q) ||
          (p.detail ?? '').toLowerCase().includes(q)),
    )
  }, [all, query, filter])
  const current = all.find((p) => principalKey(p) === active)
  const attached = (policies.data ?? []).filter((pol) =>
    current?.policy_ids.includes(pol.id),
  )

  if (principals.isPending) return <Skeleton className="h-64 w-full" />
  if (principals.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{principals.error.message}</AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
      <section className="min-w-0 space-y-3" aria-label={t('page.tabs.access')}>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t('access.search')}
            aria-label={t('access.search')}
            className="max-w-xs"
          />
          <div role="group" className="flex gap-1">
            {FILTERS.map((f) => (
              <Button
                key={f}
                type="button"
                size="sm"
                variant={filter === f ? 'default' : 'outline'}
                aria-pressed={filter === f}
                onClick={() => setFilter(f)}
              >
                {t(`access.filter.${f}`)}
              </Button>
            ))}
          </div>
        </div>
        {checked.length > 0 ? (
          <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/40 px-3 py-2">
            <span className="text-sm text-foreground">
              {t('access.selectedCount', { count: checked.length })}
            </span>
            <Button
              type="button"
              size="sm"
              onClick={() => setDialog({ mode: 'attach', keys: checked })}
            >
              {t('access.bulkAttach')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => setChecked([])}
            >
              {t('access.clearSelection')}
            </Button>
          </div>
        ) : null}
        {shown.length === 0 ? (
          <EmptyState
            icon={<UserIcon className="size-5" />}
            title={t('access.empty.title')}
            description={t('access.empty.description')}
          />
        ) : (
          <VirtualRows
            items={shown}
            label={t('page.tabs.access')}
            rowKey={principalKey}
            maxHeightClass="max-h-[28rem]"
            renderRow={(p) => {
              const k = principalKey(p)
              return (
                <PrincipalRow
                  p={p}
                  active={k === active}
                  checked={checked.includes(k)}
                  onSelect={() => setActive(k)}
                  onCheck={(on) =>
                    setChecked((c) =>
                      on ? [...c, k] : c.filter((x) => x !== k),
                    )
                  }
                />
              )
            }}
          />
        )}
      </section>

      <section className="min-w-0 space-y-4">
        {!current ? (
          <EmptyState
            icon={<UserIcon className="size-5" />}
            title={t('access.pickOne.title')}
            description={t('access.pickOne.description')}
          />
        ) : (
          <>
            <header className="space-y-1">
              <h2 className="text-base font-medium text-foreground">
                {current.name}
              </h2>
              <p className="text-xs text-muted-foreground">
                {t(`access.${current.principal_type}`)}
                {current.detail ? `, ${current.detail}` : ''}
              </p>
              <p className="text-sm text-muted-foreground">
                {t('access.flatAbilities')}:{' '}
                {current.abilities.length > 0
                  ? current.abilities.join(', ')
                  : t('effective.none')}
              </p>
            </header>
            <div className="space-y-2">
              <div className="flex items-center justify-between gap-2">
                <h3 className="text-sm font-medium text-foreground">
                  {t('access.attachedPolicies')}
                </h3>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() =>
                    setDialog({ mode: 'attach', keys: [principalKey(current)] })
                  }
                >
                  <PlusIcon />
                  {t('access.attach')}
                </Button>
              </div>
              {attached.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  {t('access.noPolicies')}
                </p>
              ) : (
                <ul className="divide-y divide-border rounded-lg border border-border">
                  {attached.map((pol) => (
                    <li
                      key={pol.id}
                      className="flex items-center justify-between gap-2 px-3 py-2"
                    >
                      <span className="min-w-0 truncate text-sm text-foreground">
                        {pol.name}
                      </span>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() =>
                          setDialog({
                            mode: 'detach',
                            policyId: pol.id,
                            keys: [principalKey(current)],
                          })
                        }
                      >
                        <LinkBreakIcon />
                        {t('detail.detach')}
                      </Button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            <EffectivePermissions
              principalType={current.principal_type}
              principalId={current.principal_id}
            />
          </>
        )}
      </section>

      {dialog ? (
        <AttachDialog
          open
          onOpenChange={(o) => {
            if (!o) {
              setDialog(null)
              setChecked([])
            }
          }}
          mode={dialog.mode}
          policies={policies.data ?? []}
          principals={all}
          policyId={dialog.policyId}
          preselected={dialog.keys}
        />
      ) : null}
    </div>
  )
}
