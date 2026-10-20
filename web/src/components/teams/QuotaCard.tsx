import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../../lib/apiError'
import {
  quotaQueryOptions,
  useClearQuota,
  useSetQuota,
  type QuotaLimits,
  type QuotaResource,
  type QuotaResourceKey,
  type QuotaScope,
} from '../../queries/quotas'

const KEYS: QuotaResourceKey[] = [
  'apps',
  'databases',
  'memory',
  'cpu',
  'previews',
]

const LIMIT_FIELD: Record<QuotaResourceKey, keyof QuotaLimits> = {
  apps: 'max_apps',
  databases: 'max_databases',
  memory: 'max_memory_mb',
  cpu: 'max_cpu',
  previews: 'max_previews',
}

function usedOf(q: QuotaResource, key: QuotaResourceKey): number {
  const u = q.usage
  return {
    apps: u.apps,
    databases: u.databases,
    memory: u.memory_mb,
    cpu: u.cpu,
    previews: u.previews,
  }[key]
}

function UsageRow({ q, k }: { q: QuotaResource; k: QuotaResourceKey }) {
  const { t } = useTranslation('teams')
  const used = usedOf(q, k)
  const limit = q.effective_limits[LIMIT_FIELD[k]]
  const pct = limit > 0 ? Math.min(100, Math.round((used / limit) * 100)) : 0
  const warn = limit > 0 && pct >= q.warn_percent
  const label = t(`quota.resources.${k}`)
  return (
    <li className="space-y-1">
      <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
        <span className="font-medium">{label}</span>
        <span className="flex items-center gap-2 text-muted-foreground">
          {limit > 0
            ? t('quota.usage', { used, limit })
            : t('quota.usageUnlimited', { used })}
          {limit > 0 && pct >= 100 ? (
            <Badge variant="destructive">{t('quota.full')}</Badge>
          ) : warn ? (
            <Badge variant="muted">{t('quota.nearLimit')}</Badge>
          ) : null}
        </span>
      </div>
      {limit > 0 && <Progress value={pct} aria-label={label} />}
    </li>
  )
}

function QuotaForm({ q, scope }: { q: QuotaResource; scope: QuotaScope }) {
  const { t } = useTranslation('teams')
  const set = useSetQuota(scope, q.scope_id)
  const clear = useClearQuota(scope, q.scope_id)
  const [draft, setDraft] = useState<Record<QuotaResourceKey, string>>(() => ({
    apps: String(q.limits.max_apps),
    databases: String(q.limits.max_databases),
    memory: String(q.limits.max_memory_mb),
    cpu: String(q.limits.max_cpu),
    previews: String(q.limits.max_previews),
  }))

  const fail = (e: unknown) => {
    toast.add({
      title:
        e instanceof ApiError
          ? e.message
          : t('errors.generic', { message: String(e) }),
      type: 'error',
    })
  }

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    const num = (k: QuotaResourceKey) => Math.max(0, Number(draft[k]) || 0)
    set.mutate(
      {
        max_apps: Math.floor(num('apps')),
        max_databases: Math.floor(num('databases')),
        max_memory_mb: Math.floor(num('memory')),
        max_cpu: num('cpu'),
        max_previews: Math.floor(num('previews')),
      },
      {
        onSuccess: () =>
          toast.add({ title: t('quota.saved'), type: 'success' }),
        onError: fail,
      },
    )
  }

  return (
    <form onSubmit={submit} className="space-y-3 border-t pt-4">
      <p className="text-sm text-muted-foreground">{t('quota.editHint')}</p>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
        {KEYS.map((k) => (
          <div key={k} className="space-y-1">
            <Label htmlFor={`quota-${scope}-${k}`}>
              {t(`quota.fields.${k}`)}
            </Label>
            <Input
              id={`quota-${scope}-${k}`}
              type="number"
              min={0}
              step={k === 'cpu' ? 0.1 : 1}
              value={draft[k]}
              onChange={(e) => {
                setDraft((d) => ({ ...d, [k]: e.target.value }))
              }}
            />
          </div>
        ))}
      </div>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={set.isPending}>
          {t('quota.save')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={clear.isPending}
          onClick={() => {
            clear.mutate(undefined, {
              onSuccess: () => {
                toast.add({ title: t('quota.cleared'), type: 'success' })
                setDraft({
                  apps: '0',
                  databases: '0',
                  memory: '0',
                  cpu: '0',
                  previews: '0',
                })
              },
              onError: fail,
            })
          }}
        >
          {t('quota.clear')}
        </Button>
      </div>
    </form>
  )
}

export function QuotaCard({
  scope,
  scopeId,
}: {
  scope: QuotaScope
  scopeId: string
}) {
  const { t } = useTranslation('teams')
  const { data: q } = useQuery(quotaQueryOptions(scope, scopeId))
  if (q === undefined) {
    return null
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('quota.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          {t(scope === 'team' ? 'quota.teamHint' : 'quota.projectHint')}
        </p>
        <ul className="space-y-3">
          {KEYS.map((k) => (
            <UsageRow key={k} q={q} k={k} />
          ))}
        </ul>
        {q.can_manage ? (
          <QuotaForm key={JSON.stringify(q.limits)} q={q} scope={scope} />
        ) : (
          <p className="text-xs text-muted-foreground">{t('quota.readOnly')}</p>
        )}
      </CardContent>
    </Card>
  )
}
