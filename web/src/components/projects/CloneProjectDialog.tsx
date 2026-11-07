import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from '@tanstack/react-router'
import { CopyGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useCloneProject,
  type CloneDomains,
  type ProjectClonePlan,
} from '../../queries/projectClone'

function Option({
  id,
  label,
  checked,
  onChange,
}: {
  id: string
  label: string
  checked: boolean
  onChange: (next: boolean) => void
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      <label htmlFor={id} className="text-sm">
        {label}
      </label>
      <Switch id={id} size="sm" checked={checked} onCheckedChange={onChange} />
    </div>
  )
}

function PlanView({ plan }: { plan: ProjectClonePlan }) {
  const { t } = useTranslation('projectClone')
  return (
    <div
      className="max-h-64 space-y-3 overflow-auto rounded-lg border border-border px-3 py-2 text-sm"
      aria-live="polite"
    >
      <div>
        <p className="font-medium">
          {t('plan.apps', { count: plan.apps.length })}
        </p>
        {plan.apps.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t('plan.empty')}</p>
        ) : (
          <ul className="text-xs text-muted-foreground">
            {plan.apps.map((app) => (
              <li key={app.name} className="truncate">
                {t('plan.appLine', { source: app.source, name: app.name })}
              </li>
            ))}
          </ul>
        )}
        <p className="text-xs text-muted-foreground">
          {t('plan.envVars', { count: plan.env_var_count })}
        </p>
      </div>
      {plan.secret_keys_needing_values.length > 0 ? (
        <div>
          <p className="font-medium">
            {t('plan.secrets', {
              count: plan.secret_keys_needing_values.length,
            })}
          </p>
          <ul className="font-mono text-xs text-muted-foreground">
            {plan.secret_keys_needing_values.map((k) => (
              <li key={`${k.scope}:${k.key}`} className="truncate">
                {k.scope}: {k.key}
              </li>
            ))}
          </ul>
          {plan.applied && plan.project.id ? (
            <Link
              to="/projects/$id"
              params={{ id: plan.project.id }}
              className="text-xs text-primary underline-offset-2 hover:underline"
            >
              {t('plan.secretsLink')}
            </Link>
          ) : null}
        </div>
      ) : null}
      {plan.skipped.length > 0 ? (
        <div>
          <p className="font-medium">
            {t('plan.skipped', { count: plan.skipped.length })}
          </p>
          <ul className="text-xs text-muted-foreground">
            {plan.skipped.map((s) => (
              <li key={`${s.scope}:${s.item}`}>
                {s.scope}: {s.item} ({s.reason})
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      <p className="text-xs text-muted-foreground">{t('plan.suspended')}</p>
    </div>
  )
}

// Clone runs a dry run first so the operator sees the plan, then applies the same request.
export function CloneProjectDialog({
  projectId,
  projectName,
  size = 'sm',
}: {
  projectId: string
  projectName: string
  size?: 'sm' | 'icon-sm'
}) {
  const { t } = useTranslation('projectClone')
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState(`${projectName}-copy`)
  const [envVars, setEnvVars] = useState(true)
  const [settings, setSettings] = useState(true)
  const [domains, setDomains] = useState<CloneDomains>('none')
  const [plan, setPlan] = useState<ProjectClonePlan | null>(null)
  const clone = useCloneProject(projectId)

  const request = (dryRun: boolean) => ({
    name: name.trim(),
    include: { env_vars: envVars, domains, settings },
    dry_run: dryRun,
  })
  const stale = () => setPlan(null)

  const run = (dryRun: boolean) => {
    clone.mutate(request(dryRun), {
      onSuccess: (result) => {
        setPlan(result)
        if (result.applied && result.project.id) {
          toast.add({ title: t('done'), type: 'success' })
          setOpen(false)
          void navigate({
            to: '/projects/$id',
            params: { id: result.project.id },
          })
        }
      },
    })
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) {
          setPlan(null)
          clone.reset()
        }
      }}
    >
      <DialogTrigger
        render={
          <Button
            variant="outline"
            size={size}
            aria-label={size === 'icon-sm' ? t('trigger') : undefined}
          />
        }
      >
        <CopyGlassIcon size="1.4em" aria-hidden="true" />
        {size === 'icon-sm' ? null : t('trigger')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-1">
          <label htmlFor="project-clone-name" className="text-sm font-medium">
            {t('name')}
          </label>
          <Input
            id="project-clone-name"
            value={name}
            onChange={(e) => {
              setName(e.target.value)
              stale()
            }}
          />
          <p className="text-xs text-muted-foreground">{t('nameHint')}</p>
        </div>
        <div className="space-y-2 rounded-lg border border-border px-3 py-2">
          <Option
            id="project-clone-env"
            label={t('options.envVars')}
            checked={envVars}
            onChange={(next) => {
              setEnvVars(next)
              stale()
            }}
          />
          <Option
            id="project-clone-settings"
            label={t('options.settings')}
            checked={settings}
            onChange={(next) => {
              setSettings(next)
              stale()
            }}
          />
          <Option
            id="project-clone-domains"
            label={`${t('options.domains')}: ${t('options.domainsPlaceholder')}`}
            checked={domains === 'placeholder'}
            onChange={(next) => {
              setDomains(next ? 'placeholder' : 'none')
              stale()
            }}
          />
        </div>
        {plan ? <PlanView plan={plan} /> : null}
        {clone.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{clone.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t('cancel')}
          </Button>
          {plan && plan.dry_run ? (
            <Button disabled={clone.isPending} onClick={() => run(false)}>
              {clone.isPending ? t('applying') : t('apply')}
            </Button>
          ) : (
            <Button
              disabled={clone.isPending || name.trim() === ''}
              onClick={() => run(true)}
            >
              {clone.isPending ? t('previewing') : t('preview')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
