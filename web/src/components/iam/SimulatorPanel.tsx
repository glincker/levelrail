import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  ListMagnifyingGlassIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { TONE } from '../kit/tone'
import {
  iamCatalogQueryOptions,
  iamPrincipalsQueryOptions,
  iamResourcesQueryOptions,
  useSimulate,
} from '../../queries/iamBuilder'
import type { Simulation, StatementRef } from '../../queries/iamBuilder'
import { EffectivePermissions } from './EffectivePermissions'

function StatementLine({
  s,
  deciding,
}: {
  s: StatementRef
  deciding?: boolean
}) {
  const { t } = useTranslation('iam')
  const deny = s.effect === 'Deny'
  const tone = deny ? TONE.danger : TONE.success
  return (
    <li
      className={cn(
        'rounded-lg border px-3 py-2 text-sm',
        deciding ? tone.border : 'border-border',
      )}
    >
      <p className="flex flex-wrap items-center gap-2">
        <span
          className={cn(
            'rounded-full px-2 py-0.5 text-xs',
            tone.soft,
            tone.text,
          )}
        >
          {deny ? t('builder.effect.deny') : t('builder.effect.allow')}
        </span>
        <span className="text-foreground">
          {t('simulator.policy', {
            name: s.policy_name,
            n: s.statement_index + 1,
          })}
        </span>
      </p>
      <p className="mt-1 text-xs text-muted-foreground">
        <code>{s.action.join(', ')}</code> on{' '}
        <code>{s.resource.join(', ')}</code>
      </p>
    </li>
  )
}

function Result({ sim }: { sim: Simulation }) {
  const { t } = useTranslation('iam')
  const tone = sim.allowed ? TONE.success : TONE.danger
  const Icon = sim.allowed ? CheckCircleIcon : XCircleIcon
  return (
    <section
      aria-live="polite"
      className={cn('space-y-3 rounded-xl border bg-card p-4', tone.border)}
    >
      <header className="flex items-start gap-3">
        <span
          className={cn(
            'flex size-9 shrink-0 items-center justify-center rounded-lg',
            tone.soft,
            tone.text,
          )}
        >
          <Icon className="size-5" aria-hidden="true" />
        </span>
        <div className="min-w-0">
          <h3 className="text-sm font-medium text-foreground">
            {sim.allowed ? t('simulator.allowed') : t('simulator.denied')}
          </h3>
          <p className="text-sm text-muted-foreground">
            {t('simulator.summary', {
              name: sim.principal_name,
              verdict: sim.allowed
                ? t('simulator.verdictAllow')
                : t('simulator.verdictDeny'),
              action: sim.action,
              resource: sim.resource,
            })}
          </p>
          <p className="mt-1 text-sm text-foreground">
            {t(`simulator.decidedBy.${sim.decided_by}`)}
          </p>
        </div>
      </header>
      {sim.deciding_statement ? (
        <div className="space-y-1.5">
          <h4 className="text-xs font-medium text-muted-foreground uppercase">
            {t('simulator.decidingStatement')}
          </h4>
          <ul>
            <StatementLine s={sim.deciding_statement} deciding />
          </ul>
        </div>
      ) : null}
      <div className="space-y-1.5">
        <h4 className="text-xs font-medium text-muted-foreground uppercase">
          {t('simulator.matched')}
        </h4>
        {sim.matched_statements.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('simulator.noneMatched')}
          </p>
        ) : (
          <ul className="space-y-1.5">
            {sim.matched_statements.map((s) => (
              <StatementLine
                key={`${s.policy_id}:${s.statement_index}`}
                s={s}
              />
            ))}
          </ul>
        )}
      </div>
      {sim.environment_resources.length > 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('simulator.environments', {
            list: sim.environment_resources.join(', '),
          })}
        </p>
      ) : null}
    </section>
  )
}

/** SimulatorPanel answers "can this principal do this ability on this resource" with the server's real evaluator and shows every statement that matched. */
export function SimulatorPanel({
  initialPrincipal,
}: {
  initialPrincipal?: string
}) {
  const { t } = useTranslation('iam')
  const principals = useQuery(iamPrincipalsQueryOptions())
  const resources = useQuery(iamResourcesQueryOptions())
  const catalog = useQuery(iamCatalogQueryOptions())
  const simulate = useSimulate()
  const [principal, setPrincipal] = useState(initialPrincipal ?? '')
  const [action, setAction] = useState('write')
  const [picked, setPicked] = useState('')
  const [typed, setTyped] = useState('')
  const [showEffective, setShowEffective] = useState(false)

  const principalOptions = useMemo(
    () =>
      (principals.data ?? []).map((p) => ({
        value: `${p.principal_type}:${p.principal_id}`,
        label: `${p.name} (${t(`access.${p.principal_type}`)})`,
      })),
    [principals.data, t],
  )
  const resourceOptions = useMemo(
    () => [
      ...(resources.data?.apps ?? []).map((a) => ({
        value: `app:${a.name}`,
        label: `app:${a.name}`,
      })),
      ...(resources.data?.databases ?? []).map((d) => ({
        value: `database:${d.name}`,
        label: `database:${d.name}`,
      })),
    ],
    [resources.data],
  )

  const resource = typed.trim() || picked
  const [ptype, ...rest] = principal.split(':')
  const pid = rest.join(':')
  const ready = Boolean(principal && resource)

  const run = () => {
    if (!ready) return
    simulate.mutate({
      principalType: ptype === 'user' ? 'user' : 'token',
      principalId: pid,
      action,
      resource,
    })
  }

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-sm font-medium text-foreground">
          {t('simulator.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('simulator.description')}
        </p>
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <Field>
          <FieldLabel>{t('simulator.principal')}</FieldLabel>
          <Combobox
            options={principalOptions}
            value={principal}
            onValueChange={setPrincipal}
            placeholder={t('simulator.principalPick')}
            searchPlaceholder={t('access.search')}
            emptyMessage={t('access.empty.title')}
            isLoading={principals.isPending}
            triggerClassName="w-full"
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="sim-action">{t('simulator.action')}</FieldLabel>
          <Select value={action} onValueChange={(v) => setAction(String(v))}>
            <SelectTrigger id="sim-action" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(catalog.data?.abilities ?? []).map((a) => (
                <SelectItem key={a.id} value={a.id}>
                  {a.id}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel>{t('simulator.resource')}</FieldLabel>
          <Combobox
            options={resourceOptions}
            value={picked}
            onValueChange={(v) => {
              setPicked(v)
              setTyped('')
            }}
            placeholder={t('simulator.resourcePick')}
            searchPlaceholder={t('builder.resources.search')}
            emptyMessage={t('builder.resources.noOptions')}
            isLoading={resources.isPending}
            triggerClassName="w-full"
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="sim-resource">
            {t('simulator.resourceCustom')}
          </FieldLabel>
          <Input
            id="sim-resource"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            placeholder="app:web"
          />
        </Field>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          onClick={run}
          disabled={!ready || simulate.isPending}
        >
          <ListMagnifyingGlassIcon />
          {simulate.isPending ? t('simulator.running') : t('simulator.run')}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={!principal}
          onClick={() => setShowEffective((v) => !v)}
          aria-expanded={showEffective}
        >
          {t('simulator.showEffective')}
        </Button>
        {!ready ? (
          <span className="text-xs text-muted-foreground">
            {t('simulator.needsInput')}
          </span>
        ) : null}
      </div>
      {simulate.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{simulate.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {simulate.data ? <Result sim={simulate.data} /> : null}
      {showEffective && principal ? (
        <EffectivePermissions principalType={ptype ?? ''} principalId={pid} />
      ) : null}
    </div>
  )
}
