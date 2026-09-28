import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
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

function volumeLabel(v: {
  name?: string
  host_path?: string
  container_path: string
}): string {
  return `${v.host_path ?? v.name ?? '(anonymous)'} -> ${v.container_path}`
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
        title: `Importing "${result.appName}".`,
        description: result.deployId
          ? 'Watching the build log now.'
          : 'It will appear in the app list shortly.',
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
          <WarningIcon aria-hidden="true" />
          <AlertTitle>Read before deploying</AlertTitle>
          <AlertDescription>
            <ul className="list-disc space-y-1 pl-4">
              {plan.warnings.map((w, i) => (
                <li key={`${w.code}-${String(i)}`}>{w.message}</li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="space-y-1 text-sm">
          <span className="font-medium">App name</span>
          <Input
            value={name}
            onChange={(e) => {
              setName(e.target.value)
            }}
            aria-invalid={!nameValid}
          />
          {!nameValid ? (
            <span className="text-xs text-destructive">
              Use lowercase letters, numbers, and hyphens only.
            </span>
          ) : null}
        </label>
        {single && plan.deploy !== 'compose' ? (
          <label className="space-y-1 text-sm">
            <span className="font-medium">Container port</span>
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
                Enter the port the app listens on.
              </span>
            ) : null}
          </label>
        ) : null}
      </div>
      {plan.domain_suggestion ? (
        <p className="text-xs text-muted-foreground">
          Domain suggestion: {plan.domain_suggestion}. Add real domains from the
          app&apos;s Domains tab after it is created.
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
              Why: {s.build_reason}
            </p>
          ) : null}
          {s.health_path ? (
            <p className="text-xs text-muted-foreground">
              Health check: GET {s.health_path}
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
                <li key={volumeLabel(v)} className="flex items-center gap-2">
                  <span className="font-mono">{volumeLabel(v)}</span>
                  {v.needs_approval ? (
                    <Badge variant="warning">Needs root approval</Badge>
                  ) : null}
                </li>
              ))}
            </ul>
          ) : null}
        </section>
      ))}

      {plan.services.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Nothing to deploy was found in this input.
        </p>
      ) : null}

      {missing.length > 0 ? (
        <p role="status" className="text-sm text-destructive">
          Required before deploying: {missing.join(', ')}
        </p>
      ) : null}
      {deploy.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{deploy.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex items-center gap-2">
        <Button type="button" variant="ghost" onClick={onBack}>
          Back
        </Button>
        <Button
          type="button"
          disabled={!canDeploy || deploy.isPending}
          onClick={() => {
            deploy.mutate()
          }}
        >
          {deploy.isPending ? 'Deploying...' : 'Deploy'}
        </Button>
      </div>
    </div>
  )
}
