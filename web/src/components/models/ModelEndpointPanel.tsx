import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RotateCwGlassIcon } from '@thesvg/ui-react'
import { RelativeTime } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import {
  useRecheckEndpoint,
  useSetEndpointCredential,
  useSetEndpointRouting,
} from '../../queries/modelEndpoints'
import { useModels } from '../../queries/models'
import type { ModelResource } from '../../types/models'
import type { ExternalEndpoint } from '../../types/modelEndpoints'
import { EndpointFix } from './EndpointFix'
import { EndpointHealthBadge } from './EndpointHealthBadge'

const NO_FALLBACK = 'none'
const POSITIVE_INT = /^[1-9][0-9]*$/

function RoutingForm({
  name,
  endpoint,
}: {
  name: string
  endpoint: ExternalEndpoint
}) {
  const { t } = useTranslation('models')
  const others = (useModels().data ?? []).filter((m) => m.name !== name)
  const [fallback, setFallback] = useState(
    endpoint.fallback_model ?? NO_FALLBACK,
  )
  const [ref, setRef] = useState(endpoint.fallback_model_ref ?? '')
  const [seconds, setSeconds] = useState(
    endpoint.latency_budget_ms > 0
      ? String(endpoint.latency_budget_ms / 1000)
      : '',
  )
  const save = useSetEndpointRouting()
  const on = fallback !== NO_FALLBACK
  const invalid = on && seconds !== '' && !POSITIVE_INT.test(seconds)

  return (
    <section aria-label={t('endpoint.routing.title')} className="space-y-3">
      <div>
        <h3 className="text-sm font-semibold">{t('endpoint.routing.title')}</h3>
        <p className="text-xs text-muted-foreground">
          {t('endpoint.routing.description')}
        </p>
      </div>
      <div className="flex flex-wrap items-end gap-3">
        <Field>
          <FieldLabel htmlFor="endpoint-fallback">
            {t('endpoint.routing.fallback')}
          </FieldLabel>
          <Select
            value={fallback}
            onValueChange={(v) => {
              if (v !== null) setFallback(v)
            }}
          >
            <SelectTrigger id="endpoint-fallback" className="w-56">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NO_FALLBACK}>
                {t('endpoint.routing.off')}
              </SelectItem>
              {others.map((m: ModelResource) => (
                <SelectItem key={m.name} value={m.name}>
                  {m.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        {on ? (
          <>
            <Field>
              <FieldLabel htmlFor="endpoint-fallback-ref">
                {t('endpoint.routing.modelId')}
              </FieldLabel>
              <Input
                id="endpoint-fallback-ref"
                value={ref}
                onChange={(e) => {
                  setRef(e.target.value)
                }}
                className="w-56 font-mono"
                placeholder={t('endpoint.routing.modelIdPlaceholder')}
                autoComplete="off"
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="endpoint-budget">
                {t('endpoint.routing.budget')}
              </FieldLabel>
              <Input
                id="endpoint-budget"
                value={seconds}
                onChange={(e) => {
                  setSeconds(e.target.value)
                }}
                className="w-28"
                inputMode="numeric"
                placeholder={t('endpoint.routing.budgetPlaceholder')}
                aria-invalid={invalid}
              />
            </Field>
          </>
        ) : null}
        <Button
          type="button"
          size="sm"
          disabled={invalid || save.isPending}
          onClick={() => {
            save.mutate(
              {
                name,
                fallbackModel: on ? fallback : '',
                fallbackModelRef: on ? ref.trim() : '',
                latencyBudgetMs:
                  on && seconds !== '' ? Number(seconds) * 1000 : 0,
              },
              {
                onSuccess: () => {
                  toast.add({
                    title: t('endpoint.routing.saved'),
                    type: 'success',
                  })
                },
                onError: (error) => {
                  toast.add({
                    title: t('endpoint.routing.saveFailed'),
                    description: error.message,
                    type: 'error',
                  })
                },
              },
            )
          }}
        >
          {t('endpoint.routing.save')}
        </Button>
      </div>
      {on ? <FieldHint>{t('endpoint.routing.hint')}</FieldHint> : null}
      {endpoint.fallback_model ? (
        <p className="text-xs text-muted-foreground">
          {t('endpoint.routing.used', { count: endpoint.fallback_count })}
        </p>
      ) : null}
    </section>
  )
}

export function ModelEndpointPanel({ model }: { model: ModelResource }) {
  const { t } = useTranslation('models')
  const recheck = useRecheckEndpoint()
  const saveCredential = useSetEndpointCredential()
  const endpoint = model.external
  if (!endpoint) return null
  const h = endpoint.health
  const online = h.state === 'online' || h.state === 'slow'

  function fail(title: string) {
    return (error: Error) => {
      toast.add({ title, description: error.message, type: 'error' })
    }
  }
  const doRecheck = () => {
    recheck.mutate(model.name, { onError: fail(t('endpoint.recheckFailed')) })
  }

  return (
    <div className="space-y-6">
      <section aria-label={t('endpoint.health.title')} className="space-y-3">
        <div className="flex flex-wrap items-center gap-3">
          <h3 className="text-sm font-semibold">
            {t('endpoint.health.title')}
          </h3>
          <EndpointHealthBadge
            state={h.state}
            latencyMs={online ? h.latency_ms : undefined}
          />
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={recheck.isPending}
            onClick={doRecheck}
          >
            <RotateCwGlassIcon size="1.4em" aria-hidden="true" />
            {recheck.isPending ? t('endpoint.testing') : t('endpoint.recheck')}
          </Button>
        </div>
        <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <div className="rounded-md border border-border p-3">
            <dt className="text-xs text-muted-foreground">
              {t('endpoint.health.upstream')}
            </dt>
            <dd className="mt-0.5 truncate font-mono text-sm">
              {endpoint.base_url}
            </dd>
          </div>
          <div className="rounded-md border border-border p-3">
            <dt className="text-xs text-muted-foreground">
              {t('endpoint.health.apiType')}
            </dt>
            <dd className="mt-0.5 text-sm">
              {t(`endpoint.apiType.${endpoint.api_type}.label`)}
            </dd>
          </div>
          <div className="rounded-md border border-border p-3">
            <dt className="text-xs text-muted-foreground">
              {t('endpoint.health.lastChecked')}
            </dt>
            <dd className="mt-0.5 text-sm">
              {h.checked_at ? <RelativeTime at={h.checked_at} /> : '-'}
            </dd>
          </div>
        </dl>
        {!endpoint.private_allowed ? (
          <p className="text-xs text-muted-foreground">
            {t('endpoint.health.privateOff')}
          </p>
        ) : null}
        {h.diagnosis ? (
          <EndpointFix
            diagnosis={h.diagnosis}
            apiType={endpoint.api_type}
            onRecheck={doRecheck}
            rechecking={recheck.isPending}
            savingCredential={saveCredential.isPending}
            onSaveCredential={(credential) => {
              saveCredential.mutate(
                { name: model.name, credential },
                { onError: fail(t('endpoint.credential.saveFailed')) },
              )
            }}
          />
        ) : null}
      </section>
      <section aria-label={t('endpoint.models.title')} className="space-y-2">
        <h3 className="text-sm font-semibold">{t('endpoint.models.title')}</h3>
        {endpoint.models.length > 0 ? (
          <ul className="flex flex-wrap gap-1.5">
            {endpoint.models.map((m) => (
              <li
                key={m}
                className="rounded-md border border-border bg-muted/40 px-2 py-0.5 font-mono text-xs"
              >
                {m}
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">
            {endpoint.discovery_error ?? t('endpoint.models.none')}
          </p>
        )}
      </section>
      <section
        aria-label={t('endpoint.credential.title')}
        className="space-y-2"
      >
        <h3 className="text-sm font-semibold">
          {t('endpoint.credential.title')}
        </h3>
        <p className="text-xs text-muted-foreground">
          {endpoint.credential_set
            ? t('endpoint.credential.set')
            : t('endpoint.credential.notSet')}
        </p>
        {endpoint.credential_set ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={saveCredential.isPending}
            onClick={() => {
              saveCredential.mutate(
                { name: model.name, credential: '' },
                {
                  onSuccess: () => {
                    toast.add({
                      title: t('endpoint.credential.removed'),
                      type: 'success',
                    })
                  },
                  onError: fail(t('endpoint.credential.saveFailed')),
                },
              )
            }}
          >
            {t('endpoint.credential.remove')}
          </Button>
        ) : null}
      </section>
      <RoutingForm
        key={`${endpoint.fallback_model ?? ''}-${String(endpoint.latency_budget_ms)}`}
        name={model.name}
        endpoint={endpoint}
      />
    </div>
  )
}
