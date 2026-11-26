import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { IMPORT_SOURCE_LABEL } from '../lib/importInput'
import {
  deployImportPlan,
  type ImportPlan,
  type ImportPlanRequest,
  type ImportService,
} from '../queries/imports'
import { appKeys } from '../queries/apps'
import { ImportEnvTable } from './ImportEnvTable'

const APP_NAME_PATTERN = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/

function requiredKeys(services: ImportService[]): string[] {
  const keys = new Set<string>()
  for (const s of services) {
    for (const e of s.env ?? []) if (e.required) keys.add(e.key)
  }
  return [...keys].sort()
}

function volumeLabel(
  v: { name?: string; host_path?: string; container_path: string },
  anonymous: string,
): string {
  return `${v.host_path ?? v.name ?? anonymous} -> ${v.container_path}`
}

/**
 * The deployment plan preview: what the import will create, with editable
 * name and port, env values with required and secret indicators, volumes and
 * warnings. One primary Deploy button reuses the existing create, build and
 * compose endpoints.
 */
export function ImportPlanPreview({
  plan,
  request,
  onBack,
  onDeployed,
}: {
  plan: ImportPlan
  request: ImportPlanRequest
  onBack: () => void
  onDeployed: () => void
}) {
  const { t } = useTranslation('migration')
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [name, setName] = useState(plan.suggested_name)
  const single = plan.services.length === 1 ? plan.services[0] : undefined
  const [port, setPort] = useState(single?.port ? String(single.port) : '')
  const [values, setValues] = useState<Record<string, string>>({})

  const missing = requiredKeys(plan.services).filter(
    (k) => (values[k] ?? '').trim() === '',
  )
  const nameValid = APP_NAME_PATTERN.test(name)
  const portNumber = Number.parseInt(port, 10)
  const portInvalid =
    single !== undefined &&
    plan.deploy !== 'compose' &&
    !(portNumber > 0 && portNumber < 65536)
  const canDeploy =
    plan.deploy !== 'none' && nameValid && missing.length === 0 && !portInvalid

  const deploy = useMutation({
    meta: { errorToast: false },
    mutationFn: () =>
      deployImportPlan(
        plan,
        {
          ...request,
          name,
          ...(single && portNumber > 0 ? { port: portNumber } : {}),
        },
        values,
      ),
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
      onDeployed()
      toast.add({
        title: t('importPlan.importingToast', { name: result.appName }),
        description: result.deployId
          ? t('importPlan.watching')
          : t('importPlan.appearsShortly'),
        type: 'success',
      })
      if (result.deployId) {
        void navigate({
          to: '/apps/$name/deploys/$deployId/logs',
          params: { name: result.appName, deployId: result.deployId },
        })
      } else {
        void navigate({ to: '/apps/$name', params: { name: result.appName } })
      }
    },
  })

  return (
    <div className="space-y-4" data-testid="import-plan">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="outline">{IMPORT_SOURCE_LABEL[plan.source]}</Badge>
        {plan.repo_url ? (
          <span className="font-mono text-xs text-muted-foreground">
            {plan.repo_url}
            {plan.ref ? ` (${plan.ref})` : ''}
          </span>
        ) : null}
      </div>

      {plan.warnings.length > 0 ? (
        <Alert>
          <TriangleAlertGlassIcon
            className="text-tone-warning"
            size="1.7em"
            aria-hidden="true"
          />
          <AlertTitle>{t('importPlan.readFirst')}</AlertTitle>
          <AlertDescription>
            <ul className="list-disc space-y-1 pl-4">
              {plan.warnings.map((w, i) => (
                <li key={`${w.code}-${String(i)}`}>{w.message}</li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      ) : null}

      {plan.sample ? (
        <div className="space-y-1" data-testid="import-sample">
          <Badge variant="muted">{t('importPlan.sampleBadge')}</Badge>
          <p className="text-xs text-muted-foreground">
            {t('importPlan.sampleHint')}
          </p>
          <pre
            className="max-h-48 overflow-auto rounded-md border border-border bg-muted/40 p-2 font-mono text-xs"
            aria-label={t('importPlan.sampleLogLabel')}
          >
            {plan.sample.buildLog.join('\n')}
          </pre>
        </div>
      ) : null}

      {(plan.resources ?? []).length > 0 ? (
        <div className="space-y-1 text-sm" data-testid="import-resources">
          <p className="font-medium">{t('importPlan.willCreate')}</p>
          <ul className="space-y-0.5 text-xs text-muted-foreground">
            {(plan.resources ?? []).map((r) => (
              <li key={`${r.kind}-${r.name}`}>
                <Badge variant="outline" className="mr-2">
                  {r.kind}
                </Badge>
                <span className="font-mono">{r.name}</span>
                {r.detail ? ` (${r.detail})` : ''}
              </li>
            ))}
          </ul>
          {(plan.secret_refs ?? []).length > 0 ? (
            <p className="text-xs text-muted-foreground">
              {t('importPlan.secretRefs', {
                keys: (plan.secret_refs ?? []).join(', '),
              })}
            </p>
          ) : null}
        </div>
      ) : null}

      {(plan.unsupported ?? []).length > 0 ? (
        <Alert data-testid="import-unsupported">
          <TriangleAlertGlassIcon
            className="text-tone-warning"
            size="1.7em"
            aria-hidden="true"
          />
          <AlertTitle>{t('importPlan.unsupported')}</AlertTitle>
          <AlertDescription>
            <ul className="list-disc space-y-1 pl-4">
              {(plan.unsupported ?? []).map((u) => (
                <li key={`${u.service ?? ''}-${u.feature}`}>
                  {u.service ? `${u.service} ` : ''}
                  {u.feature}: {u.reason}.{' '}
                  {u.alternative
                    ? t('importPlan.instead', { alternative: u.alternative })
                    : ''}
                </li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      ) : null}

      {(plan.conflicts ?? []).length > 0 ? (
        <Alert variant="destructive" data-testid="import-conflicts">
          <AlertTitle>{t('importPlan.conflicts')}</AlertTitle>
          <AlertDescription>
            {(plan.conflicts ?? [])
              .map((c) => `${c.kind} ${c.name}`)
              .join(', ')}
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="space-y-1 text-sm">
          <span className="font-medium">{t('importPlan.appName')}</span>
          <Input
            value={name}
            onChange={(e) => {
              setName(e.target.value)
            }}
            aria-invalid={!nameValid}
          />
          {!nameValid ? (
            <span className="text-xs text-destructive">
              {t('importPlan.nameInvalid')}
            </span>
          ) : null}
        </label>
        {single && plan.deploy !== 'compose' ? (
          <label className="space-y-1 text-sm">
            <span className="font-medium">{t('importPlan.containerPort')}</span>
            <Input
              inputMode="numeric"
              value={port}
              onChange={(e) => {
                setPort(e.target.value.replace(/\D/g, ''))
              }}
              aria-invalid={portInvalid}
            />
            {portInvalid ? (
              <span className="text-xs text-destructive">
                {t('importPlan.portInvalid')}
              </span>
            ) : null}
          </label>
        ) : null}
      </div>
      {plan.domain_suggestion ? (
        <p className="text-xs text-muted-foreground">
          {t('importPlan.domainSuggestion', { domain: plan.domain_suggestion })}
        </p>
      ) : null}

      {plan.services.map((s) => (
        <section
          key={s.name}
          className="space-y-2 rounded-lg border border-border p-3"
        >
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="text-sm font-medium">{s.name}</h4>
            <Badge variant="default">{s.build}</Badge>
            {s.image ? (
              <span className="font-mono text-xs text-muted-foreground">
                {s.image}
              </span>
            ) : null}
          </div>
          {s.build_reason ? (
            <p className="text-xs text-muted-foreground">
              {t('importPlan.why', { reason: s.build_reason })}
            </p>
          ) : null}
          {s.health_path ? (
            <p className="text-xs text-muted-foreground">
              {t('importPlan.health', { path: s.health_path })}
            </p>
          ) : null}
          <ImportEnvTable
            service={s}
            values={values}
            onChange={(key, value) => {
              setValues((prev) => ({ ...prev, [key]: value }))
            }}
          />
          {(s.volumes ?? []).length > 0 ? (
            <ul className="space-y-1 text-xs">
              {(s.volumes ?? []).map((v) => (
                <li
                  key={volumeLabel(v, '')}
                  className="flex items-center gap-2"
                >
                  <span className="font-mono">
                    {volumeLabel(v, t('importPlan.anonymousVolume'))}
                  </span>
                  {v.needs_approval ? (
                    <Badge variant="warning">
                      {t('importPlan.needsApproval')}
                    </Badge>
                  ) : null}
                </li>
              ))}
            </ul>
          ) : null}
        </section>
      ))}

      {plan.services.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('importPlan.nothing')}
        </p>
      ) : null}

      {missing.length > 0 ? (
        <p role="status" className="text-sm text-destructive">
          {t('importPlan.required', { keys: missing.join(', ') })}
        </p>
      ) : null}
      {deploy.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{deploy.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex items-center gap-2">
        <Button type="button" variant="ghost" onClick={onBack}>
          {t('importPlan.back')}
        </Button>
        <Button
          type="button"
          disabled={!canDeploy || deploy.isPending}
          onClick={() => {
            deploy.mutate()
          }}
        >
          {deploy.isPending
            ? t('importPlan.deploying')
            : t('importPlan.deploy')}
        </Button>
      </div>
    </div>
  )
}
