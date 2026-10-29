import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { LightbulbGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import type {
  AnalyzeDatabase,
  AnalyzeEnvKey,
} from '../../queries/importAnalyze'
import type { DatabaseEdit } from '../../lib/newApp'

/** One pre-filled suggestion: the editable control plus the evidence for it. */
export function Finding({
  id,
  label,
  evidence,
  children,
}: {
  id: string
  label: string
  evidence: string
  children: ReactNode
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {evidence ? (
        <p className="flex items-start gap-1.5 text-xs text-muted-foreground">
          <LightbulbGlassIcon
            className="mt-0.5 size-4 shrink-0"
            aria-hidden="true"
          />
          <span>{evidence}</span>
        </p>
      ) : null}
    </div>
  )
}

export function DatabaseFinding({
  need,
  edit,
  onChange,
}: {
  need: AnalyzeDatabase
  edit: DatabaseEdit
  onChange: (next: DatabaseEdit) => void
}) {
  const { t } = useTranslation('newApp')
  const switchId = `db-${need.engine}-enabled`
  return (
    <div className="space-y-2 rounded-md border border-border p-3">
      <div className="flex items-center justify-between gap-3">
        <Label htmlFor={switchId} className="flex items-center gap-2">
          {t('findings.databases.attach', {
            engine: need.engine,
            version: edit.version,
          })}
        </Label>
        <Switch
          id={switchId}
          checked={edit.enabled}
          onCheckedChange={(enabled) => {
            onChange({ ...edit, enabled })
          }}
        />
      </div>
      {edit.enabled ? (
        <div className="grid gap-2 sm:grid-cols-2">
          <div className="space-y-1">
            <Label htmlFor={`${switchId}-name`}>
              {t('findings.databases.name')}
            </Label>
            <Input
              id={`${switchId}-name`}
              value={edit.name}
              onChange={(e) => {
                onChange({ ...edit, name: e.target.value })
              }}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor={`${switchId}-env`}>
              {t('findings.databases.envVar')}
            </Label>
            <Input
              id={`${switchId}-env`}
              value={edit.envVar}
              className="font-mono text-xs"
              onChange={(e) => {
                onChange({ ...edit, envVar: e.target.value })
              }}
            />
          </div>
        </div>
      ) : null}
      <p className="flex items-start gap-1.5 text-xs text-muted-foreground">
        <LightbulbGlassIcon
          className="mt-0.5 size-4 shrink-0"
          aria-hidden="true"
        />
        <span>{need.evidence}</span>
      </p>
    </div>
  )
}

export function EnvFinding({
  entry,
  value,
  onChange,
}: {
  entry: AnalyzeEnvKey
  value: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation('newApp')
  const provided = entry.provided_by !== undefined && entry.provided_by !== ''
  const id = `env-${entry.key}`
  return (
    <div className="grid gap-1.5 sm:grid-cols-[minmax(0,14rem)_1fr] sm:items-start">
      <div className="min-w-0 space-y-1">
        <Label htmlFor={id} className="font-mono text-xs break-all">
          {entry.key}
        </Label>
        <div className="flex flex-wrap gap-1">
          {entry.required && !provided ? (
            <Badge variant="warning">{t('findings.env.required')}</Badge>
          ) : null}
          {entry.secret ? (
            <Badge variant="muted">{t('findings.env.secret')}</Badge>
          ) : null}
        </div>
      </div>
      <div className="space-y-1">
        {provided ? (
          <p className="text-xs text-muted-foreground">
            {t('findings.env.provided', {
              source: entry.provided_by?.replace(/^database:/, ''),
            })}
          </p>
        ) : (
          <Input
            id={id}
            type={entry.secret ? 'password' : 'text'}
            autoComplete="off"
            value={value}
            placeholder={
              entry.has_default
                ? t('findings.env.hasDefault')
                : t('findings.env.placeholder')
            }
            onChange={(e) => {
              onChange(e.target.value)
            }}
          />
        )}
        <p className="text-xs text-muted-foreground">
          {t('findings.env.source', { source: entry.source })}
        </p>
      </div>
    </div>
  )
}
