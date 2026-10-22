import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CheckCircleIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useDatabaseNameFree } from '../../hooks/useDatabaseNameFree'
import { ApiError, ExistingVolumeError } from '../../lib/apiError'
import { ApiErrorAlert } from '../ApiErrorAlert'
import { useDockerDegradedPoll } from '../../queries/dockerIntegration'
import { useBackupTargetsOptional } from '../../queries/backupTargets'
import { useDatabaseEnginesOptional } from '../../queries/databaseEngines'
import { useCreateDatabase, useDatabases } from '../../queries/databases'
import { useDatabaseVersions } from '../../queries/databaseVersions'
import { useSystemStatusOptional } from '../../queries/systemStatus'
import { ExistingVolumePrompt } from '../ExistingVolumePrompt'
import { DatabaseVersionPicker } from '../databaseVersions/DatabaseVersionPicker'
import { Advanced, LOCAL_NODE, NO_PROJECT } from './Advanced'
import { EngineCards, PresetCards } from './Pickers'
import {
  CREDENTIALED_ENGINES,
  DEFAULT_ENGINE,
  FALLBACK_ENGINES,
  PRESETS,
  SCHEDULES,
  SCHEDULE_LABEL,
  backupRetain,
  defaultPreset,
  pickVersion,
  presetResources,
  scheduleCron,
  suggestName,
  summaryKey,
  supportsBackups,
  validate,
  type DbValues,
  type ScheduleId,
} from './logic'

function Hint({ children }: Readonly<{ children: React.ReactNode }>) {
  return <p className="text-xs text-muted-foreground">{children}</p>
}

export function DatabaseFlow({ onClose }: Readonly<{ onClose: () => void }>) {
  const { t } = useTranslation('databases')
  const navigate = useNavigate()
  const engineQuery = useDatabaseEnginesOptional()
  const engines = engineQuery.data?.length ? engineQuery.data : FALLBACK_ENGINES
  const existing = useDatabases()
  const taken = (existing.data ?? []).map((d) => d.name)
  const targets = useBackupTargetsOptional().data ?? []
  const systemStatus = useSystemStatusOptional()
  const create = useCreateDatabase()

  const initialEngine = engines[0]?.id ?? DEFAULT_ENGINE.id
  const [base, setValues] = useState<DbValues>({
    engine: initialEngine,
    version: '',
    name: '',
    backups: true,
    schedule: 'daily',
    preset: defaultPreset(initialEngine),
    nodeId: LOCAL_NODE,
    projectId: NO_PROJECT,
  })
  const [userName, setUserName] = useState<string | null>(null)
  const [userVersion, setUserVersion] = useState<string | null>(null)
  const [attempted, setAttempted] = useState(false)
  const [nameBlurred, setNameBlurred] = useState(false)

  const engine = engines.find((e) => e.id === base.engine) ?? DEFAULT_ENGINE
  const versions = useDatabaseVersions(base.engine).data?.versions ?? []
  const values: DbValues = {
    ...base,
    version:
      userVersion ?? pickVersion('', false, versions, engine.default_version),
    name: userName ?? suggestName(base.engine, taken),
  }
  const target = targets[0]
  const backupsAvailable = target !== undefined

  const errors = validate(values, taken)
  const nameFree = useDatabaseNameFree(values.name, !errors.name)
  const serverTaken = nameFree === false
  const nameError = errors.name ?? (serverTaken ? 'taken' : undefined)
  const showName = attempted || nameBlurred
  const showVersion = attempted && errors.version

  function patch(next: Partial<DbValues>) {
    setValues((prev) => ({ ...prev, ...next }))
  }

  function chooseEngine(id: string) {
    if (id === values.engine) return
    setUserVersion(null)
    patch({ engine: id, preset: defaultPreset(id) })
  }

  async function submit(volume?: 'reuse' | 'discard') {
    const name = values.name.trim()
    try {
      await create.mutateAsync({
        name,
        engine: values.engine,
        version: values.version.trim(),
        project_id:
          values.projectId === NO_PROJECT ? undefined : values.projectId,
        node_id: values.nodeId === LOCAL_NODE ? undefined : values.nodeId,
        existing_volume: volume,
        resources: presetResources(values.preset),
        ...(target && values.backups && supportsBackups(values.engine)
          ? {
              backup_target_id: target.id,
              backup_schedule: scheduleCron(values.schedule),
              backup_retain: backupRetain(),
            }
          : {}),
      })
    } catch {
      return
    }
    toast.add({ title: t('flow.created', { name }), type: 'success' })
    onClose()
    void navigate({ to: '/databases/$name', params: { name } })
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (Object.keys(errors).length > 0 || serverTaken) {
      setAttempted(true)
      return
    }
    void submit()
  }

  const dockerDown = useDockerDegradedPoll().data?.unresponsive === true
  const dockerDownError = new ApiError(503, t('flow.dockerDown'), {
    code: 'docker_unavailable',
  })
  const pending = create.isPending
  const volumeError =
    create.error instanceof ExistingVolumeError ? create.error : null
  const ready = values.name.trim() !== '' && values.version.trim() !== ''
  const summary = t(summaryKey(values, backupsAvailable), {
    engine: engine.label,
    version: values.version.trim(),
    name: values.name.trim(),
    memory: PRESETS.find((p) => p.id === values.preset)?.memoryMiB,
    schedule: t(SCHEDULE_LABEL[values.schedule]).toLowerCase(),
  })

  return (
    <form
      noValidate
      onSubmit={onSubmit}
      className="flex min-h-0 min-w-0 flex-1 flex-col gap-4"
    >
      <DialogHeader>
        <DialogTitle>{t('flow.title')}</DialogTitle>
        <DialogDescription>{t('flow.description')}</DialogDescription>
      </DialogHeader>
      <div className="-mx-1 min-h-0 min-w-0 flex-1 space-y-5 overflow-y-auto px-1">
        <section className="min-w-0 space-y-2">
          <h3 className="text-sm font-medium">{t('flow.engine')}</h3>
          <EngineCards
            engines={engines}
            value={values.engine}
            onChange={chooseEngine}
          />
          {CREDENTIALED_ENGINES.has(values.engine) &&
          systemStatus.data?.secrets_configured === false ? (
            <Alert>
              <WarningIcon aria-hidden="true" />
              <AlertDescription>
                {t('flow.secretsWarning')}{' '}
                <Link
                  to="/settings/general"
                  className="underline underline-offset-2"
                >
                  {t('flow.secretsLink')}
                </Link>
              </AlertDescription>
            </Alert>
          ) : null}
        </section>

        <div className="grid min-w-0 gap-4 sm:grid-cols-2">
          <div className="min-w-0 space-y-1.5">
            <Label htmlFor="database-name">{t('flow.name')}</Label>
            <Input
              id="database-name"
              className="font-mono"
              value={values.name}
              autoComplete="off"
              autoCapitalize="off"
              spellCheck={false}
              aria-invalid={showName && nameError ? true : undefined}
              aria-describedby="database-name-msg"
              onChange={(e) => {
                setUserName(e.target.value)
              }}
              onBlur={() => {
                setNameBlurred(true)
              }}
            />
            <div id="database-name-msg">
              {showName && nameError ? (
                <p role="alert" className="text-xs text-destructive">
                  {t(`flow.errors.name.${nameError}`)}
                </p>
              ) : nameFree ? (
                <p className="flex items-center gap-1 text-xs text-muted-foreground">
                  <CheckCircleIcon aria-hidden="true" className="size-3.5" />
                  {t('flow.nameFree')}
                </p>
              ) : (
                <Hint>{t('flow.nameHint')}</Hint>
              )}
            </div>
          </div>
          <div className="min-w-0 space-y-1.5">
            <Label htmlFor="database-version">{t('flow.version')}</Label>
            <DatabaseVersionPicker
              id="database-version"
              engine={values.engine}
              value={values.version}
              placeholder={engine.default_version}
              onChange={(version) => {
                setUserVersion(version)
              }}
            />
            {showVersion ? (
              <p role="alert" className="text-xs text-destructive">
                {t('flow.errors.version.required')}
              </p>
            ) : (
              <Hint>{t('flow.versionHint')}</Hint>
            )}
          </div>
        </div>

        <section className="min-w-0 space-y-2">
          <h3 className="text-sm font-medium">{t('flow.size')}</h3>
          <PresetCards
            value={values.preset}
            onChange={(preset) => {
              patch({ preset })
            }}
          />
          <Hint>{t('flow.sizeHint')}</Hint>
        </section>

        {supportsBackups(values.engine) ? (
          <section className="min-w-0 space-y-3 rounded-lg border border-border p-3">
            <div className="flex min-w-0 items-start justify-between gap-3">
              <div className="min-w-0">
                <Label htmlFor="database-backups">{t('flow.backups')}</Label>
                <Hint>
                  {backupsAvailable ? (
                    t('flow.backupsHint')
                  ) : (
                    <>
                      {t('flow.noTarget')}{' '}
                      <Link
                        to="/settings/backup-targets"
                        className="underline underline-offset-2"
                      >
                        {t('flow.noTargetLink')}
                      </Link>
                    </>
                  )}
                </Hint>
              </div>
              <Switch
                id="database-backups"
                checked={values.backups && backupsAvailable}
                disabled={!backupsAvailable}
                onCheckedChange={(backups) => {
                  patch({ backups })
                }}
              />
            </div>
            {values.backups && backupsAvailable ? (
              <div className="min-w-0 space-y-1.5">
                <Label htmlFor="database-schedule">{t('flow.schedule')}</Label>
                <Select
                  value={values.schedule}
                  onValueChange={(v) => {
                    patch({ schedule: v ?? 'daily' })
                  }}
                >
                  <SelectTrigger id="database-schedule" className="w-full">
                    <SelectValue>
                      {(v: ScheduleId) => t(SCHEDULE_LABEL[v])}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {SCHEDULES.map((s) => (
                      <SelectItem key={s.id} value={s.id}>
                        {t(SCHEDULE_LABEL[s.id])}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            ) : null}
          </section>
        ) : null}

        <Advanced
          nodeId={values.nodeId}
          projectId={values.projectId}
          onNode={(nodeId) => {
            patch({ nodeId })
          }}
          onProject={(projectId) => {
            patch({ projectId })
          }}
        />

        {volumeError ? (
          <ExistingVolumePrompt
            volumes={volumeError.volumes}
            pending={pending}
            onChoose={(choice) => {
              void submit(choice)
            }}
          />
        ) : create.isError ? (
          <ApiErrorAlert error={create.error} />
        ) : null}
        {dockerDown ? <ApiErrorAlert error={dockerDownError} /> : null}
      </div>
      <p
        data-testid="database-summary"
        className="min-w-0 break-words rounded-md bg-muted px-3 py-2 font-mono text-xs text-muted-foreground"
      >
        {ready ? summary : t('flow.summary.empty')}
      </p>
      <DialogFooter className="sm:flex-wrap">
        <Button type="button" variant="outline" onClick={onClose}>
          {t('flow.cancel')}
        </Button>
        <Button type="submit" disabled={pending}>
          {pending ? t('flow.submitting') : t('flow.submit')}
        </Button>
      </DialogFooter>
    </form>
  )
}
