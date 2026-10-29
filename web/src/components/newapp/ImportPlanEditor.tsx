import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  CircleCheckGlassIcon,
  PlusGlassIcon,
  RocketGlassIcon,
  RotateCwGlassIcon,
  SlidersHorizontalGlassIcon,
  TrashGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SecretInput } from '@/components/ui/secret-input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useEnvironmentListOptional } from '../../queries/environments'
import {
  applyRepoImport,
  type ImportApplyPackage,
  type ImportApplyResponse,
  type ImportApplyResult,
  type ImportApplyStatus,
} from '../../queries/importRepoApply'
import type { RepoPlan, RepoPlanRow } from '../../queries/importRepoPlan'
import { useProjectListOptional } from '../../queries/projects'
import type { GitRepoSourceValue } from '../GitRepoSourcePicker'

interface EnvEntry {
  id: number
  key: string
  value: string
  secret: boolean
}

interface RowState {
  row: RepoPlanRow
  include: boolean
  name: string
  port: string
  env: EnvEntry[]
}

type Stage = 'edit' | 'review' | 'results'

const NONE = '__none__'
const STATUS_TONE: Record<
  ImportApplyStatus,
  'success' | 'muted' | 'warning' | 'destructive' | 'info'
> = {
  created: 'success',
  already_exists: 'muted',
  skipped_dependency: 'warning',
  failed: 'destructive',
  would_create: 'info',
}

let nextEnvId = 1

function initialRows(plan: RepoPlan): RowState[] {
  return (plan.packages ?? []).map((row) => ({
    row,
    include: true,
    name: row.name,
    port: row.port === null ? '' : String(row.port),
    env: [],
  }))
}

/**
 * Editable review of an import plan: pick packages, adjust name, port and
 * env, run a dry run, then create the apps server side and show per-app
 * results with a retry for failures.
 */
export function ImportPlanEditor({
  plan,
  source,
}: {
  plan: RepoPlan
  source: GitRepoSourceValue
}) {
  const { t } = useTranslation('newApp')
  const [rows, setRows] = useState<RowState[]>(() => initialRows(plan))
  const [stage, setStage] = useState<Stage>('edit')
  const [projectPick, setProjectPick] = useState<string | null>(null)
  const [envPick, setEnvPick] = useState<string | null>(null)
  const [deploy, setDeploy] = useState(false)
  const [envOf, setEnvOf] = useState<string | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [dry, setDry] = useState<ImportApplyResponse | null>(null)
  const [results, setResults] = useState<ImportApplyResult[]>([])

  const projects = useProjectListOptional().data ?? []
  const projectId =
    projectPick ?? (projects.length === 1 ? (projects[0]?.id ?? '') : '')
  const environments = useEnvironmentListOptional(projectId).data ?? []
  const open = environments.filter((e) => !e.protected)
  const environmentId =
    envPick ?? (open.length === 1 ? (open[0]?.id ?? '') : '')

  const mutation = useMutation({ mutationFn: applyRepoImport })
  const included = rows.filter((r) => r.include)

  function update(directory: string, patch: Partial<RowState>) {
    setRows((cur) =>
      cur.map((r) => (r.row.directory === directory ? { ...r, ...patch } : r)),
    )
  }

  function toPackage(r: RowState): ImportApplyPackage {
    const env: Record<string, string> = {}
    const secretKeys: string[] = []
    for (const e of r.env) {
      const key = e.key.trim()
      if (key === '') continue
      env[key] = e.value
      if (e.secret) secretKeys.push(key)
    }
    const port = Number.parseInt(r.port, 10)
    return {
      package: r.row.package_name,
      name: r.name.trim(),
      directory: r.row.directory,
      ...(Number.isNaN(port) ? {} : { port }),
      ...(Object.keys(env).length > 0
        ? { env, secret_env_keys: secretKeys }
        : {}),
      ...(projectId !== '' ? { project_id: projectId } : {}),
      ...(environmentId !== '' ? { environment_id: environmentId } : {}),
      ...(deploy ? { deploy: true } : {}),
    }
  }

  function request(
    list: RowState[],
    opts: { dryRun: boolean; confirm: boolean },
  ) {
    return {
      repo_url: plan.repo_url,
      branch: plan.branch,
      base_directory: plan.base_directory,
      token: source.token,
      packages: list.map(toPackage),
      dry_run: opts.dryRun,
      confirm: opts.confirm,
    }
  }

  function runDry() {
    mutation.mutate(request(included, { dryRun: true, confirm: false }), {
      onSuccess: (res) => {
        setDry(res)
        setStage('review')
      },
    })
  }

  function runApply(list: RowState[], confirm: boolean) {
    mutation.mutate(request(list, { dryRun: false, confirm }), {
      onSuccess: (res) => {
        setConfirmOpen(false)
        setResults((cur) => {
          const byDir = new Map(cur.map((r) => [r.directory, r]))
          for (const r of res.results) byDir.set(r.directory, r)
          return [...byDir.values()]
        })
        setStage('results')
      },
    })
  }

  const wouldCreate = dry?.counts.would_create ?? 0
  const failedRows = results.filter(
    (r) => r.status === 'failed' || r.status === 'skipped_dependency',
  )

  function startCreate() {
    if (dry?.needs_confirmation) setConfirmOpen(true)
    else runApply(included, false)
  }

  function retryFailed() {
    const dirs = new Set(failedRows.map((r) => r.directory))
    runApply(
      rows.filter((r) => dirs.has(r.row.directory)),
      true,
    )
  }

  const errorText = mutation.isError ? mutation.error.message : null
  const envRow = rows.find((r) => r.row.directory === envOf) ?? null

  return (
    <div className="space-y-3">
      {errorText ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('plan.editor.error', { message: errorText })}
          </AlertDescription>
        </Alert>
      ) : null}

      {stage === 'edit' ? (
        <>
          <div className="flex flex-wrap items-end gap-3">
            <ScopeSelect
              id="f-plan-project"
              label={t('plan.editor.project')}
              noneLabel={t('plan.editor.noProject')}
              value={projectId}
              options={projects.map((p) => ({ value: p.id, label: p.name }))}
              onChange={(v) => {
                setProjectPick(v)
                setEnvPick('')
              }}
            />
            <ScopeSelect
              id="f-plan-env"
              label={t('plan.editor.environment')}
              noneLabel={t('plan.editor.noEnvironment')}
              value={environmentId}
              options={environments.map((e) => ({
                value: e.id,
                label: e.protected
                  ? t('plan.editor.protectedEnv', { name: e.name })
                  : e.name,
                disabled: e.protected,
              }))}
              onChange={setEnvPick}
            />
            <div className="flex items-center gap-2 pb-1.5">
              <Checkbox
                id="f-plan-deploy"
                checked={deploy}
                onCheckedChange={(v) => {
                  setDeploy(v)
                }}
              />
              <Label htmlFor="f-plan-deploy" className="text-xs">
                {t('plan.editor.deploy')}
              </Label>
            </div>
          </div>
          <div className="overflow-x-auto rounded-md border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-8">
                    <span className="sr-only">
                      {t('plan.editor.columns.include')}
                    </span>
                  </TableHead>
                  <TableHead>{t('plan.columns.name')}</TableHead>
                  <TableHead>{t('plan.columns.directory')}</TableHead>
                  <TableHead className="w-24">
                    {t('plan.columns.port')}
                  </TableHead>
                  <TableHead>{t('plan.columns.build')}</TableHead>
                  <TableHead>{t('plan.columns.dependsOn')}</TableHead>
                  <TableHead>{t('plan.editor.columns.env')}</TableHead>
                  <TableHead>{t('plan.columns.warnings')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((r) => (
                  <TableRow key={r.row.directory}>
                    <TableCell>
                      <Checkbox
                        checked={r.include}
                        aria-label={t('plan.editor.includeRow', {
                          name: r.name,
                        })}
                        onCheckedChange={(v) => {
                          update(r.row.directory, { include: v })
                        }}
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        value={r.name}
                        aria-label={t('plan.editor.nameOf', {
                          directory: r.row.directory,
                        })}
                        onChange={(e) => {
                          update(r.row.directory, { name: e.target.value })
                        }}
                      />
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {r.row.directory}
                    </TableCell>
                    <TableCell>
                      <Input
                        inputMode="numeric"
                        value={r.port}
                        aria-label={t('plan.editor.portOf', { name: r.name })}
                        onChange={(e) => {
                          update(r.row.directory, { port: e.target.value })
                        }}
                      />
                    </TableCell>
                    <TableCell>
                      <Badge variant="muted">
                        {t(`plan.kinds.${r.row.build_kind}`)}
                      </Badge>
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {r.row.depends_on.length > 0
                        ? r.row.depends_on.join(', ')
                        : t('plan.none')}
                    </TableCell>
                    <TableCell>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          setEnvOf(r.row.directory)
                        }}
                      >
                        <SlidersHorizontalGlassIcon
                          className="size-5"
                          aria-hidden
                        />
                        {t('plan.editor.envButton', { count: r.env.length })}
                      </Button>
                    </TableCell>
                    <TableCell>
                      {r.row.warnings.length === 0 ? (
                        t('plan.none')
                      ) : (
                        <ul className="space-y-0.5 text-xs">
                          {r.row.warnings.map((w) => (
                            <li key={w.code} className="flex items-start gap-1">
                              <TriangleAlertGlassIcon
                                className="mt-0.5 size-4.5 shrink-0 text-tone-warning"
                                aria-hidden
                              />
                              {t(`plan.warnings.${w.code}`, {
                                defaultValue: w.message,
                              })}
                            </li>
                          ))}
                        </ul>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <Button
              type="button"
              size="sm"
              disabled={included.length === 0 || mutation.isPending}
              onClick={runDry}
            >
              <RocketGlassIcon className="size-5" aria-hidden />
              {mutation.isPending
                ? t('plan.editor.checking')
                : t('plan.editor.review', { count: included.length })}
            </Button>
            <p className="text-xs text-muted-foreground">
              {t('plan.editor.reviewHint')}
            </p>
          </div>
        </>
      ) : null}

      {stage === 'review' && dry ? (
        <>
          <p className="text-xs text-muted-foreground">
            {t('plan.editor.dryRunSummary', {
              create: wouldCreate,
              problems: dry.counts.failed ?? 0,
            })}
          </p>
          <ResultsTable results={dry.results} linkApps={false} />
          <div className="flex flex-wrap items-center gap-3">
            <Button
              type="button"
              size="sm"
              disabled={wouldCreate === 0 || mutation.isPending}
              onClick={startCreate}
            >
              <RocketGlassIcon className="size-5" aria-hidden />
              {mutation.isPending
                ? t('plan.editor.creating')
                : t('plan.editor.create', { count: wouldCreate })}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                setStage('edit')
              }}
            >
              {t('plan.editor.backToEdit')}
            </Button>
          </div>
        </>
      ) : null}

      {stage === 'results' ? (
        <>
          <p className="flex items-center gap-1 text-xs text-muted-foreground">
            <CircleCheckGlassIcon
              className="size-5 text-tone-success"
              aria-hidden
            />
            {t('plan.editor.resultsSummary', {
              created: results.filter((r) => r.status === 'created').length,
              failed: failedRows.length,
            })}
          </p>
          <ResultsTable results={results} linkApps />
          <div className="flex flex-wrap items-center gap-3">
            {failedRows.length > 0 ? (
              <Button
                type="button"
                size="sm"
                disabled={mutation.isPending}
                onClick={retryFailed}
              >
                <RotateCwGlassIcon className="size-5" aria-hidden />
                {t('plan.editor.retryFailed', { count: failedRows.length })}
              </Button>
            ) : null}
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                setStage('edit')
              }}
            >
              {t('plan.editor.backToEdit')}
            </Button>
          </div>
        </>
      ) : null}

      <Dialog
        open={envRow !== null}
        onOpenChange={(o) => {
          if (!o) setEnvOf(null)
        }}
      >
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>
              {t('plan.editor.envTitle', { name: envRow?.name ?? '' })}
            </DialogTitle>
            <DialogDescription>{t('plan.editor.envHint')}</DialogDescription>
          </DialogHeader>
          {envRow ? (
            <EnvEditor
              entries={envRow.env}
              onChange={(env) => {
                update(envRow.row.directory, { env })
              }}
            />
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              size="sm"
              onClick={() => {
                setEnvOf(null)
              }}
            >
              {t('plan.editor.envDone')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('plan.editor.confirm.title', { count: wouldCreate })}
        description={t('plan.editor.confirm.description', {
          count: wouldCreate,
          above: dry?.confirm_above ?? 0,
        })}
        confirmLabel={t('plan.editor.confirm.confirm', { count: wouldCreate })}
        pendingLabel={t('plan.editor.creating')}
        pending={mutation.isPending}
        error={errorText}
        onConfirm={() => {
          runApply(included, true)
        }}
      />
    </div>
  )
}

function ScopeSelect({
  id,
  label,
  noneLabel,
  value,
  options,
  onChange,
}: {
  id: string
  label: string
  noneLabel: string
  value: string
  options: { value: string; label: string; disabled?: boolean }[]
  onChange: (v: string) => void
}) {
  return (
    <div className="space-y-1">
      <Label htmlFor={id} className="text-xs">
        {label}
      </Label>
      <Select
        value={value === '' ? NONE : value}
        onValueChange={(v) => {
          onChange(v === NONE ? '' : (v ?? ''))
        }}
      >
        <SelectTrigger id={id} className="w-48">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NONE}>{noneLabel}</SelectItem>
          {options.map((o) => (
            <SelectItem key={o.value} value={o.value} disabled={o.disabled}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

function EnvEditor({
  entries,
  onChange,
}: {
  entries: EnvEntry[]
  onChange: (next: EnvEntry[]) => void
}) {
  const { t } = useTranslation('newApp')
  const patch = (id: number, p: Partial<EnvEntry>) => {
    onChange(entries.map((e) => (e.id === id ? { ...e, ...p } : e)))
  }
  return (
    <div className="space-y-2">
      {entries.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('plan.editor.envEmpty')}
        </p>
      ) : null}
      {entries.map((e) => (
        <div key={e.id} className="flex items-center gap-2">
          <Input
            className="font-mono"
            value={e.key}
            placeholder={t('plan.editor.envKey')}
            aria-label={t('plan.editor.envKey')}
            onChange={(ev) => {
              patch(e.id, { key: ev.target.value })
            }}
          />
          {e.secret ? (
            <SecretInput
              value={e.value}
              placeholder={t('plan.editor.envValue')}
              aria-label={t('plan.editor.envValue')}
              onChange={(ev) => {
                patch(e.id, { value: ev.target.value })
              }}
            />
          ) : (
            <Input
              value={e.value}
              placeholder={t('plan.editor.envValue')}
              aria-label={t('plan.editor.envValue')}
              onChange={(ev) => {
                patch(e.id, { value: ev.target.value })
              }}
            />
          )}
          <Checkbox
            checked={e.secret}
            aria-label={t('plan.editor.envSecret')}
            onCheckedChange={(v) => {
              patch(e.id, { secret: v })
            }}
          />
          <Button
            type="button"
            size="icon"
            variant="ghost"
            aria-label={t('plan.editor.envRemove')}
            onClick={() => {
              onChange(entries.filter((x) => x.id !== e.id))
            }}
          >
            <TrashGlassIcon className="size-5" aria-hidden />
          </Button>
        </div>
      ))}
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => {
          onChange([
            ...entries,
            { id: nextEnvId++, key: '', value: '', secret: false },
          ])
        }}
      >
        <PlusGlassIcon className="size-5" aria-hidden />
        {t('plan.editor.envAdd')}
      </Button>
    </div>
  )
}

function ResultsTable({
  results,
  linkApps,
}: {
  results: ImportApplyResult[]
  linkApps: boolean
}) {
  const { t } = useTranslation('newApp')
  return (
    <div className="overflow-x-auto rounded-md border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('plan.columns.name')}</TableHead>
            <TableHead>{t('plan.columns.directory')}</TableHead>
            <TableHead>{t('plan.editor.columns.status')}</TableHead>
            <TableHead>{t('plan.editor.columns.detail')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {results.map((r) => (
            <TableRow key={r.directory}>
              <TableCell className="font-medium">
                {linkApps &&
                (r.status === 'created' || r.status === 'already_exists') ? (
                  <Link to="/apps/$name" params={{ name: r.name }}>
                    {r.name}
                  </Link>
                ) : (
                  r.name
                )}
              </TableCell>
              <TableCell className="font-mono text-xs">{r.directory}</TableCell>
              <TableCell>
                <Badge variant={STATUS_TONE[r.status]}>
                  {t(`plan.editor.status.${r.status}`)}
                </Badge>
              </TableCell>
              <TableCell className="text-xs">
                {r.message ?? ''}
                {r.deploy_error
                  ? ` ${t('plan.editor.deployError', { message: r.deploy_error })}`
                  : ''}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
