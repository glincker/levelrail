import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  usePutAppImportPlan,
  type AppImportCheck,
  type AppImportMapping,
  type AppImportSession,
} from '../queries/appImport'
import { AppImportConnect } from './AppImportConnect'

const statusVariant = {
  pass: 'success',
  warn: 'warning',
  fail: 'destructive',
} as const

function Mappings({ session }: { session: AppImportSession }) {
  const { t } = useTranslation('migration')
  const put = usePutAppImportPlan()
  const [rows, setRows] = useState<AppImportMapping[]>(session.mappings)
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const dirty = JSON.stringify(rows) !== JSON.stringify(session.mappings)
  const hasSuggestion = (m: AppImportMapping) =>
    rows.some((r) => r.from === m.from)

  return (
    <div className="space-y-3 rounded-md border p-3">
      <div className="space-y-1">
        <h4 className="text-sm font-semibold">
          {t('appImport.preflight.mappingTitle')}
        </h4>
        <p className="text-xs text-muted-foreground">
          {t('appImport.preflight.mappingHelp')}
        </p>
      </div>
      {session.suggested_mappings.length > 0 ? (
        <div className="space-y-1">
          <p className="text-xs font-medium">
            {t('appImport.preflight.suggested')}
          </p>
          <ul className="space-y-1">
            {session.suggested_mappings.map((m) => (
              <li
                key={m.from}
                className="flex flex-wrap items-center gap-2 text-sm"
              >
                <code className="text-xs">
                  {m.from} {'->'} {m.to}
                </code>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={hasSuggestion(m)}
                  onClick={() => setRows([...rows, m])}
                >
                  {t('appImport.preflight.use')}
                </Button>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      <ul className="space-y-1.5">
        {rows.map((m, i) => (
          <li
            key={`${m.from}-${i}`}
            className="flex flex-wrap items-center gap-2"
          >
            <code className="text-xs">
              {m.from} {'->'} {m.to}
            </code>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setRows(rows.filter((_, j) => j !== i))}
            >
              {t('appImport.preflight.remove')}
            </Button>
          </li>
        ))}
      </ul>
      <div className="flex flex-wrap items-end gap-2">
        <Input
          aria-label={t('appImport.preflight.from')}
          placeholder={t('appImport.preflight.from')}
          value={from}
          onChange={(e) => setFrom(e.target.value)}
          className="h-8 w-56"
        />
        <Input
          aria-label={t('appImport.preflight.to')}
          placeholder={t('appImport.preflight.to')}
          value={to}
          onChange={(e) => setTo(e.target.value)}
          className="h-8 w-56"
        />
        <Button
          size="sm"
          variant="outline"
          disabled={!from.trim() || !to.trim()}
          onClick={() => {
            setRows([...rows, { from: from.trim(), to: to.trim() }])
            setFrom('')
            setTo('')
          }}
        >
          {t('appImport.preflight.add')}
        </Button>
        <Button
          size="sm"
          disabled={!dirty || put.isPending}
          onClick={() =>
            put.mutate({ id: session.id, update: { mappings: rows } })
          }
        >
          {put.isPending
            ? t('appImport.preflight.saving')
            : t('appImport.preflight.save')}
        </Button>
      </div>
      {put.isError ? (
        <p className="text-sm text-destructive">{put.error.message}</p>
      ) : null}
    </div>
  )
}

function Checks({ checks }: { checks: AppImportCheck[] }) {
  const { t } = useTranslation('migration')
  const shown = checks.filter((c) => c.status !== 'pass')
  const passed = checks.length - shown.length
  return (
    <div className="space-y-2">
      <p className="text-sm text-muted-foreground">
        {t('appImport.preflight.passed', { count: passed })}
      </p>
      <ul className="space-y-2">
        {shown.map((c, i) => (
          <li
            key={`${c.app}-${c.id}-${i}`}
            className="rounded-md border p-2 text-sm"
          >
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={statusVariant[c.status]}>{c.status}</Badge>
              {c.app ? <span className="font-medium">{c.app}</span> : null}
              <span className="text-xs text-muted-foreground">
                {t(`appImport.preflight.check.${c.id}`, { defaultValue: c.id })}
              </span>
            </div>
            <p className="mt-1">{c.detail}</p>
            {c.fix ? (
              <p className="text-xs text-muted-foreground">{c.fix}</p>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  )
}

function Diff({ session }: { session: AppImportSession }) {
  const { t } = useTranslation('migration')
  const diff = session.preflight?.diff ?? []
  if (diff.length === 0) return null
  return (
    <div className="space-y-2">
      <h4 className="text-sm font-semibold">{t('appImport.preflight.diff')}</h4>
      <p className="text-xs text-muted-foreground">
        {t('appImport.preflight.diffHelp')}
      </p>
      <ul className="space-y-2">
        {diff.map((d) => (
          <li
            key={`${d.app}-${d.key}`}
            className="rounded-md border p-2 text-xs"
          >
            <p className="font-medium">
              {d.app} {d.key}
              {d.secret ? (
                <Badge variant="muted" className="ml-2">
                  {t('appImport.preflight.secret')}
                </Badge>
              ) : null}
            </p>
            <p className="mt-1 break-all font-mono text-destructive">
              - {d.before}
            </p>
            <p className="break-all font-mono text-green-700 dark:text-green-400">
              + {d.after}
            </p>
          </li>
        ))}
      </ul>
    </div>
  )
}

export function AppImportPreflight({
  session,
  onBack,
  onNext,
}: {
  session: AppImportSession
  onBack: () => void
  onNext: () => void
}) {
  const { t } = useTranslation('migration')
  const put = usePutAppImportPlan()
  const pre = session.preflight
  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">
          {t('appImport.preflight.title')}
        </h3>
        <p className="text-sm text-muted-foreground">
          {t('appImport.preflight.description')}
        </p>
      </div>
      {!session.connected ? (
        <Alert>
          <AlertDescription>
            {t('appImport.preflight.needsToken')}
            <div className="mt-3">
              <AppImportConnect
                reconnect={{
                  id: session.id,
                  url: session.source_url,
                  platform: session.platform,
                }}
                onCreated={() => undefined}
              />
            </div>
          </AlertDescription>
        </Alert>
      ) : (
        <>
          <Mappings key={JSON.stringify(session.mappings)} session={session} />
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span>{t('appImport.preflight.collision')}</span>
            {(['suffix', 'skip'] as const).map((c) => (
              <Button
                key={c}
                size="sm"
                variant={session.collision === c ? 'default' : 'outline'}
                disabled={put.isPending}
                onClick={() =>
                  put.mutate({ id: session.id, update: { collision: c } })
                }
              >
                {t(`appImport.preflight.collisionMode.${c}`)}
              </Button>
            ))}
          </div>
          {pre ? <Checks checks={pre.checks} /> : null}
          <Diff session={session} />
        </>
      )}
      <div className="flex items-center gap-2">
        <Button variant="outline" onClick={onBack}>
          {t('appImport.back')}
        </Button>
        <Button disabled={!pre?.can_stage} onClick={onNext}>
          {t('appImport.preflight.continue')}
        </Button>
        {pre && !pre.can_stage ? (
          <span className="text-xs text-destructive">
            {t('appImport.preflight.blocked', { count: pre.failed })}
          </span>
        ) : null}
      </div>
    </div>
  )
}
