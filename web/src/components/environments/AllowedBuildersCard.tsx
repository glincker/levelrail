import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { HammerGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { RemovableChip } from '@/components/ui/removable-chip'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { InfoTip } from '../kit/InfoTip'
import { useBuildRunners } from '../../queries/buildRunners'
import {
  BUILD_TRUST_TIERS,
  isBuildTrustTier,
  useEnvironmentBuilders,
  useSaveEnvironmentBuilders,
  type EnvironmentBuilders,
} from '../../queries/builderTrust'

function sameSet(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((x) => b.includes(x))
}

function BuildersForm({
  envId,
  view,
  canEdit,
}: Readonly<{ envId: string; view: EnvironmentBuilders; canEdit: boolean }>) {
  const { t } = useTranslation('nodes')
  const { data } = useBuildRunners()
  const save = useSaveEnvironmentBuilders(envId)
  const [allowed, setAllowed] = useState<string[]>(view.allowed_builders)
  const nodes = (data?.runners ?? []).filter((r) => r.kind === 'node')
  const nodeName = (id: string) => nodes.find((n) => n.id === id)?.name ?? id
  const nodeIds = allowed.filter((a) => !isBuildTrustTier(a))
  const pickable = nodes.filter((n) => !allowed.includes(n.id))
  const dirty = !sameSet(allowed, view.allowed_builders)
  const isProduction = view.kind === 'production'
  const busy = save.isPending

  const toggleTier = (tier: string) => {
    setAllowed((cur) =>
      cur.includes(tier) ? cur.filter((x) => x !== tier) : [...cur, tier],
    )
  }
  const onSave = (next: string[]) => {
    save.mutate(next, {
      onSuccess: (saved) => {
        setAllowed(saved.allowed_builders)
        toast.add({ title: t('allowedBuilders.saved'), type: 'success' })
      },
    })
  }

  return (
    <div className="space-y-3">
      <div
        role="group"
        aria-label={t('allowedBuilders.tiers')}
        className="flex flex-wrap items-center gap-2"
      >
        {BUILD_TRUST_TIERS.map((tier) => {
          const on = allowed.includes(tier)
          return (
            <button
              key={tier}
              type="button"
              aria-pressed={on}
              disabled={!canEdit || busy}
              onClick={() => {
                toggleTier(tier)
              }}
              className={cn(
                'inline-flex h-7 items-center rounded-glinr-pill border px-3 text-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-state-accent disabled:opacity-50',
                on
                  ? 'border-tone-accent-border bg-tone-accent-soft text-tone-accent'
                  : 'border-border bg-muted/60 text-muted-foreground hover:text-foreground',
              )}
            >
              {t(`builderTrust.tier.${tier}`)}
            </button>
          )
        })}
        {nodeIds.map((id) => (
          <RemovableChip
            key={id}
            label={nodeName(id)}
            onRemove={() => {
              setAllowed((cur) => cur.filter((x) => x !== id))
            }}
          />
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Label htmlFor={`env-builder-node-${envId}`} className="sr-only">
          {t('allowedBuilders.addNode')}
        </Label>
        <NativeSelect
          id={`env-builder-node-${envId}`}
          className="w-52"
          value=""
          disabled={!canEdit || busy || pickable.length === 0}
          onChange={(e) => {
            const id = e.target.value
            if (id) setAllowed((cur) => [...cur, id])
          }}
        >
          <option value="">{t('allowedBuilders.addNode')}</option>
          {pickable.map((n) => (
            <option key={n.id} value={n.id}>
              {n.name}
            </option>
          ))}
        </NativeSelect>
        <p className="text-xs text-muted-foreground">
          {allowed.length === 0
            ? t('allowedBuilders.defaultPolicy')
            : t('allowedBuilders.customPolicy')}
        </p>
        {isProduction && allowed.length === 0 ? (
          <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
            {view.personal_allowed
              ? t('allowedBuilders.personalOn')
              : t('allowedBuilders.personalOff')}
            <InfoTip label={t('allowedBuilders.productionTipLabel')}>
              {t('allowedBuilders.productionTip')}
            </InfoTip>
          </span>
        ) : null}
      </div>
      {save.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <div className="flex items-center justify-end gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={!canEdit || busy || view.allowed_builders.length === 0}
          onClick={() => {
            onSave([])
          }}
        >
          {t('allowedBuilders.reset')}
        </Button>
        <Button
          size="sm"
          disabled={!canEdit || busy || !dirty}
          onClick={() => {
            onSave(allowed)
          }}
        >
          {t('allowedBuilders.save')}
        </Button>
      </div>
    </div>
  )
}

// Which builders may build this environment. Empty means the default policy:
// server and managed always, personal only outside production.
export function AllowedBuildersCard({
  environmentId,
  canEdit = true,
}: Readonly<{ environmentId: string; canEdit?: boolean }>) {
  const { t } = useTranslation('nodes')
  const { data, isPending, isError, error } =
    useEnvironmentBuilders(environmentId)
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <HammerGlassIcon aria-hidden="true" className="size-4" />
          {t('allowedBuilders.title')}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {isPending ? <Skeleton className="h-16 w-full" /> : null}
        {isError ? (
          <Alert variant="destructive">
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        ) : null}
        {data ? (
          <BuildersForm
            key={data.allowed_builders.join(',')}
            envId={environmentId}
            view={data}
            canEdit={canEdit}
          />
        ) : null}
      </CardContent>
    </Card>
  )
}
