import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { CopySimpleIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { toast } from '@/components/ui/toast'
import {
  useCloneEnvironment,
  useEnvironmentClonePreview,
} from '../queries/environmentClone'
import type { EnvironmentCloneAppInput } from '../types/environmentClone'
import { ApiError } from '../lib/apiError'

// Per-app override state, keyed by source app name: an empty newName
// means "use the server's own auto-suggested name", an empty
// domainsText means "assign no domains", the same defaults GET
// .../clone/preview already documents.
interface AppOverride {
  newName: string
  domainsText: string
}

function parseDomains(text: string): string[] {
  return text
    .split(',')
    .map((d) => d.trim())
    .filter((d) => d !== '')
}

// CloneEnvironmentDialog is "apps environments clone"
// (cmd/levelrail-cli/apps_environments_clone.go) and POST/GET
// /api/v1/environments/{id}/clone[/preview]
// (internal/api/environment_clone.go) surfaced on the dashboard: type a
// name for the new environment, see exactly which apps would be cloned
// (with their own suggested new names, current domains that will NOT be
// carried over, and declared secret env vars), optionally override a
// name or assign domains per app, make an explicit secrets choice, then
// confirm. Domains and secret values are never copied silently, the
// same "be careful crossing a tier boundary" precedent promote.go's own
// preview note already sets for env vars.
export function CloneEnvironmentDialog({
  environmentId,
  environmentName,
  projectId,
}: {
  environmentId: string
  environmentName: string
  projectId: string
}) {
  const [open, setOpen] = useState(false)
  const [newName, setNewName] = useState('')
  const [copySecretValues, setCopySecretValues] = useState(false)
  const [overrides, setOverrides] = useState<Record<string, AppOverride>>({})
  const navigate = useNavigate()

  const preview = useEnvironmentClonePreview(environmentId, newName)
  const clone = useCloneEnvironment(projectId)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setNewName('')
      setCopySecretValues(false)
      setOverrides({})
      clone.reset()
    }
  }

  function overrideFor(sourceApp: string): AppOverride {
    return overrides[sourceApp] ?? { newName: '', domainsText: '' }
  }

  function setOverride(sourceApp: string, patch: Partial<AppOverride>) {
    setOverrides((prev) => ({
      ...prev,
      [sourceApp]: { ...overrideFor(sourceApp), ...patch },
    }))
  }

  function handleClone() {
    const apps: EnvironmentCloneAppInput[] = Object.entries(overrides)
      .map(([sourceApp, o]) => ({
        source_app: sourceApp,
        new_name: o.newName.trim() || undefined,
        domains: parseDomains(o.domainsText),
      }))
      .filter((a) => a.new_name !== undefined || (a.domains?.length ?? 0) > 0)

    clone.mutate(
      {
        id: environmentId,
        input: { newEnvironmentName: newName.trim(), copySecretValues, apps },
      },
      {
        onSuccess: (result) => {
          setOpen(false)
          toast.add({
            title: `Environment "${result.environment.name}" created.`,
            description: `${result.apps.length} app${result.apps.length === 1 ? '' : 's'} cloned and deploying.`,
            type: 'success',
          })
          void navigate({
            to: '/projects/$id/environments/$envId',
            params: { id: projectId, envId: result.environment.id },
          })
        },
      },
    )
  }

  const previewErrorMessage =
    preview.error instanceof ApiError ? preview.error.message : undefined
  const apps = preview.data?.apps ?? []
  const hasSecrets =
    apps.some((a) => a.secret_env_keys.length > 0) ||
    (preview.data?.environment_secret_env_keys.length ?? 0) > 0

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button size="sm" variant="outline" />}>
        <CopySimpleIcon className="size-3.5" aria-hidden="true" />
        Clone environment...
      </DialogTrigger>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Clone &ldquo;{environmentName}&rdquo;</DialogTitle>
          <DialogDescription>
            Creates a new environment plus a real, deployed copy of every app
            tagged with this one (env vars, resources, health checks, volumes,
            and more). Domains and secret values are never carried over
            silently, see below.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <Field>
            <FieldLabel htmlFor="clone-new-environment-name">
              New environment name
            </FieldLabel>
            <Input
              id="clone-new-environment-name"
              placeholder="e.g. staging-eu"
              value={newName}
              onChange={(e) => {
                setNewName(e.target.value)
              }}
              autoFocus
            />
          </Field>

          {newName.trim() && preview.isPending ? (
            <p className="text-sm text-muted-foreground">Loading preview...</p>
          ) : null}

          {newName.trim() && previewErrorMessage ? (
            <p className="flex items-start gap-1.5 text-sm text-destructive">
              <WarningIcon
                className="mt-0.5 size-4 shrink-0"
                aria-hidden="true"
              />
              {previewErrorMessage}
            </p>
          ) : null}

          {preview.data && apps.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No apps are tagged with this environment; there is nothing to
              clone.
            </p>
          ) : null}

          {preview.data && apps.length > 0 ? (
            <div className="max-h-64 space-y-3 overflow-y-auto rounded-md border border-border p-3">
              {apps.map((a) => {
                const o = overrideFor(a.source_app)
                return (
                  <div
                    key={a.source_app}
                    className="space-y-2 border-b border-border pb-2 last:border-0 last:pb-0"
                  >
                    <div className="flex flex-wrap items-center gap-1.5 text-sm">
                      <span className="font-medium text-foreground">
                        {a.source_app}
                      </span>
                      <span
                        className="text-muted-foreground"
                        aria-hidden="true"
                      >
                        &rarr;
                      </span>
                      <span className="font-mono text-xs text-muted-foreground">
                        {o.newName.trim() || a.suggested_new_name}
                      </span>
                      {a.current_domains.length > 0 ? (
                        <Badge variant="outline" className="text-xs">
                          {a.current_domains.length} domain
                          {a.current_domains.length === 1 ? '' : 's'} not copied
                        </Badge>
                      ) : null}
                      {a.secret_env_keys.length > 0 ? (
                        <Badge variant="outline" className="text-xs">
                          {a.secret_env_keys.length} secret env var
                          {a.secret_env_keys.length === 1 ? '' : 's'}
                        </Badge>
                      ) : null}
                      {a.scheduled_task_count > 0 ? (
                        <Badge variant="outline" className="text-xs">
                          {a.scheduled_task_count} scheduled task
                          {a.scheduled_task_count === 1 ? '' : 's'}
                        </Badge>
                      ) : null}
                    </div>
                    <div className="grid grid-cols-2 gap-2">
                      <Input
                        aria-label={`New name for ${a.source_app}`}
                        placeholder={a.suggested_new_name}
                        value={o.newName}
                        onChange={(e) => {
                          setOverride(a.source_app, { newName: e.target.value })
                        }}
                      />
                      <Input
                        aria-label={`Domains for ${a.source_app}`}
                        placeholder="new domains, comma-separated"
                        value={o.domainsText}
                        onChange={(e) => {
                          setOverride(a.source_app, {
                            domainsText: e.target.value,
                          })
                        }}
                      />
                    </div>
                  </div>
                )
              })}
            </div>
          ) : null}

          {preview.data ? (
            <label className="flex items-start gap-2 text-sm">
              <Checkbox
                checked={copySecretValues}
                onCheckedChange={(checked) => {
                  setCopySecretValues(checked === true)
                }}
              />
              <span>
                Also copy real secret values onto the clone
                {hasSecrets ? '' : ' (no secrets declared here yet)'}. Leave
                unchecked to declare every secret with no value set, the same as
                a brand-new required secret.
              </span>
            </label>
          ) : null}

          {preview.data ? <FieldHint>{preview.data.note}</FieldHint> : null}
        </div>

        {clone.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{clone.error.message}</AlertDescription>
          </Alert>
        ) : null}

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            disabled={
              newName.trim() === '' ||
              !preview.data ||
              apps.length === 0 ||
              clone.isPending
            }
            onClick={handleClone}
          >
            {clone.isPending ? 'Cloning...' : 'Clone environment'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
