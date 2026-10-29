import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DatabaseGlassIcon, LockGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { useDatabaseNameFree } from '../../../hooks/useDatabaseNameFree'
import { editsFromAnalysis, type DatabaseEdit } from '../../../lib/newApp'
import { useBackupTargetsOptional } from '../../../queries/backupTargets'
import { useDatabaseEnginesOptional } from '../../../queries/databaseEngines'
import { useDatabases } from '../../../queries/databases'
import { useDatabaseVersions } from '../../../queries/databaseVersions'
import type { ImportAnalysis } from '../../../queries/importAnalyze'
import {
  FALLBACK_ENGINES,
  pickVersion,
  supportsBackups,
  validateName,
} from '../../databaseFlow/logic'
import {
  chipEngines,
  envVarFor,
  existingDatabaseEdit,
  injectedVars,
  newDatabaseEdit,
  suggestDbName,
} from './databaseSectionLogic'

type Mode = 'new' | 'existing'

/**
 * Optional "Needs a database?" card. Reports the databases Deploy should
 * create or attach: null leaves the analysis defaults untouched.
 */
export function DatabaseSection({
  analysis,
  appName,
  onChange,
}: {
  analysis: ImportAnalysis
  appName: string
  onChange: (databases: DatabaseEdit[] | null) => void
}) {
  const { t } = useTranslation('newApp')
  const detected = analysis.databases[0]
  const [open, setOpen] = useState(detected !== undefined)
  const [mode, setMode] = useState<Mode>('new')
  const [engine, setEngine] = useState<string | null>(detected?.engine ?? null)
  const [cleared, setCleared] = useState(false)
  const [userName, setUserName] = useState<string | null>(null)
  const [userBackups, setUserBackups] = useState<boolean | null>(null)
  const [existingName, setExistingName] = useState('')

  const engineData = useDatabaseEnginesOptional().data
  const registry = Array.isArray(engineData) ? engineData : []
  const chips = chipEngines(registry, FALLBACK_ENGINES)
  const listed = useDatabases().data
  const existing = Array.isArray(listed) ? listed : []
  const taken = existing.map((d) => d.name)
  const target = useBackupTargetsOptional().data?.[0]
  const info = [...registry, ...FALLBACK_ENGINES].find((e) => e.id === engine)
  const versions = useDatabaseVersions(engine ?? '').data?.versions ?? []
  const version = pickVersion(
    '',
    false,
    versions,
    detected?.engine === engine && detected.version
      ? detected.version
      : (info?.default_version ?? ''),
  )
  const name =
    userName ??
    (detected && detected.engine === engine
      ? detected.suggested_name
      : suggestDbName(appName, engine ?? '', taken))
  const canBackup = engine !== null && supportsBackups(engine)
  const backups = (userBackups ?? target !== undefined) && canBackup

  const nameError = validateName(name, taken)
  const nameFree = useDatabaseNameFree(
    name,
    mode === 'new' && engine !== null && !nameError,
  )
  const nameMessage = nameError ?? (nameFree === false ? 'taken' : null)

  const extras = editsFromAnalysis(analysis).databases.slice(1)
  const existingPick = existing.find((d) => d.name === existingName)

  useEffect(() => {
    let mine: DatabaseEdit | null = null
    if (mode === 'existing' && existingPick) {
      mine = existingDatabaseEdit(existingPick)
    } else if (mode === 'new' && engine !== null && nameMessage === null) {
      mine = newDatabaseEdit({
        engine,
        version,
        name,
        backups,
        target,
        ...(detected && detected.engine === engine
          ? { envVar: detected.env_var, field: detected.field || 'url' }
          : {}),
      })
    }
    if (mine) onChange([mine, ...extras])
    else if (cleared) onChange([])
    else onChange(null)
    // extras derives from analysis, which is stable for the form's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, engine, version, name, backups, target?.id, existingPick, cleared])

  const chosen: DatabaseEdit[] = [
    ...(mode === 'existing' && existingPick
      ? [existingDatabaseEdit(existingPick)]
      : []),
    ...(mode === 'new' && engine !== null
      ? [{ ...newDatabaseEdit({ engine, version, name, backups, target }) }]
      : []),
    ...extras,
  ]
  const vars = injectedVars(chosen)

  function pick(id: string) {
    setMode('new')
    setCleared(false)
    setUserName(null)
    setEngine(engine === id ? null : id)
    if (engine === id) setCleared(true)
  }

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className="min-w-0 rounded-lg border border-border">
        <CollapsibleTrigger className="flex w-full items-center gap-2 p-3 text-left text-sm font-medium text-foreground">
          <DatabaseGlassIcon aria-hidden="true" className="size-4.5 shrink-0" />
          {t('importDatabase.title')}
          <span className="font-normal text-muted-foreground">
            {t('importDatabase.optional')}
          </span>
        </CollapsibleTrigger>
        <CollapsiblePanel className="space-y-4 px-3 pb-3">
          {detected ? (
            <p className="text-sm text-muted-foreground">
              {t('importDatabase.detected', { engine: detected.engine })}
            </p>
          ) : null}
          <div
            role="group"
            aria-label={t('importDatabase.engines')}
            className="flex flex-wrap gap-2"
          >
            {chips.map((c) => (
              <Button
                key={c.id}
                type="button"
                size="sm"
                variant={
                  mode === 'new' && engine === c.id ? 'default' : 'outline'
                }
                aria-pressed={mode === 'new' && engine === c.id}
                onClick={() => {
                  pick(c.id)
                }}
              >
                {c.label}
              </Button>
            ))}
            <Button
              type="button"
              size="sm"
              variant={mode === 'existing' ? 'default' : 'outline'}
              aria-pressed={mode === 'existing'}
              onClick={() => {
                setMode(mode === 'existing' ? 'new' : 'existing')
                setCleared(false)
              }}
            >
              {t('importDatabase.useExisting')}
            </Button>
          </div>

          {mode === 'new' && engine !== null ? (
            <div className="min-w-0 space-y-3">
              <p className="text-sm text-muted-foreground" aria-live="polite">
                {t(
                  backups
                    ? 'importDatabase.summaryBackups'
                    : 'importDatabase.summary',
                  {
                    engine: info?.label ?? engine,
                    version,
                    envVar:
                      detected && detected.engine === engine
                        ? detected.env_var
                        : envVarFor(engine),
                  },
                )}
              </p>
              <Field>
                <FieldLabel htmlFor="repo-db-name">
                  {t('importDatabase.name')}
                </FieldLabel>
                <Input
                  id="repo-db-name"
                  value={name}
                  autoComplete="off"
                  spellCheck={false}
                  aria-invalid={nameMessage !== null}
                  onChange={(e) => {
                    setUserName(e.target.value.toLowerCase())
                  }}
                />
                <FieldDescription>
                  {nameMessage !== null
                    ? t(`importDatabase.nameError.${nameMessage}`)
                    : t('importDatabase.nameFree')}
                </FieldDescription>
              </Field>
              {canBackup ? (
                <label className="flex min-w-0 items-center gap-2 text-sm">
                  <Checkbox
                    checked={backups}
                    disabled={target === undefined}
                    onCheckedChange={(v) => {
                      setUserBackups(v === true)
                    }}
                  />
                  <span className="min-w-0">
                    {target === undefined
                      ? t('importDatabase.noDestination')
                      : t('importDatabase.backups')}
                  </span>
                </label>
              ) : null}
            </div>
          ) : null}

          {mode === 'existing' ? (
            <Field>
              <FieldLabel htmlFor="repo-db-existing">
                {t('importDatabase.existingLabel')}
              </FieldLabel>
              {existing.length === 0 ? (
                <FieldDescription>
                  {t('importDatabase.existingNone')}
                </FieldDescription>
              ) : (
                <select
                  id="repo-db-existing"
                  value={existingName}
                  className={cn(
                    'h-9 w-full min-w-0 rounded-md border border-input bg-background px-2 text-sm',
                  )}
                  onChange={(e) => {
                    setExistingName(e.target.value)
                  }}
                >
                  <option value="">{t('importDatabase.existingPick')}</option>
                  {existing.map((d) => (
                    <option key={d.name} value={d.name}>
                      {d.name} ({d.engine})
                    </option>
                  ))}
                </select>
              )}
            </Field>
          ) : null}

          {vars.length > 0 ? (
            <ul
              className="min-w-0 space-y-1"
              aria-label={t('importDatabase.injected')}
            >
              {vars.map((v) => (
                <li
                  key={v.key}
                  className="flex min-w-0 items-center gap-2 text-xs"
                >
                  <LockGlassIcon aria-hidden="true" className="size-4" />
                  <code className="font-mono">{v.key}</code>
                  <span aria-hidden="true">••••••••</span>
                  <span className="min-w-0 truncate text-muted-foreground">
                    {t('importDatabase.from', { database: v.database })}
                  </span>
                </li>
              ))}
            </ul>
          ) : null}
        </CollapsiblePanel>
      </div>
    </Collapsible>
  )
}
