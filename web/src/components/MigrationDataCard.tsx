import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  useDatabaseCopies,
  useStartDatabaseCopy,
  type CopyStatus,
  type DatabaseCopy,
} from '../queries/migration'

const statusVariant: Record<
  CopyStatus,
  'success' | 'destructive' | 'muted' | 'warning'
> = {
  pending: 'muted',
  copying: 'warning',
  verified: 'success',
  failed: 'destructive',
  unsupported: 'muted',
}

function CopyForm({ db, onDone }: { db: DatabaseCopy; onDone: () => void }) {
  const { t } = useTranslation('migration')
  const start = useStartDatabaseCopy()
  const [host, setHost] = useState(db.source_host ?? '')
  const [port, setPort] = useState('')
  const [user, setUser] = useState('')
  const [password, setPassword] = useState('')
  const [database, setDatabase] = useState(db.source_database ?? '')
  const [authDatabase, setAuthDatabase] = useState('')
  const [tls, setTls] = useState(false)
  const id = `copy-${db.database}`

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    start.mutate(
      {
        name: db.database,
        source: {
          host: host.trim(),
          port: port ? Number(port) : undefined,
          user: user || undefined,
          password: password || undefined,
          database: database || undefined,
          auth_database: authDatabase || undefined,
          tls: tls || undefined,
        },
      },
      {
        onSuccess: () => {
          setPassword('')
          onDone()
        },
      },
    )
  }

  return (
    <form onSubmit={onSubmit} className="mt-3 grid gap-3 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor={`${id}-host`}>{t('data.fields.host')}</Label>
        <Input
          id={`${id}-host`}
          required
          value={host}
          onChange={(e) => setHost(e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`${id}-port`}>{t('data.fields.port')}</Label>
        <Input
          id={`${id}-port`}
          type="number"
          min={1}
          max={65535}
          value={port}
          onChange={(e) => setPort(e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`${id}-user`}>{t('data.fields.user')}</Label>
        <Input
          id={`${id}-user`}
          autoComplete="off"
          value={user}
          onChange={(e) => setUser(e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`${id}-password`}>{t('data.fields.password')}</Label>
        <Input
          id={`${id}-password`}
          type="password"
          autoComplete="off"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <p className="text-xs text-muted-foreground">
          {t('data.passwordHelp')}
        </p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`${id}-db`}>{t('data.fields.database')}</Label>
        <Input
          id={`${id}-db`}
          value={database}
          onChange={(e) => setDatabase(e.target.value)}
        />
      </div>
      {db.engine === 'mongodb' ? (
        <div className="space-y-1.5">
          <Label htmlFor={`${id}-auth`}>{t('data.fields.authDatabase')}</Label>
          <Input
            id={`${id}-auth`}
            value={authDatabase}
            onChange={(e) => setAuthDatabase(e.target.value)}
          />
        </div>
      ) : null}
      <Label className="flex items-center gap-2 font-normal sm:col-span-2">
        <Checkbox
          aria-label={t('data.fields.tls')}
          checked={tls}
          onCheckedChange={(v) => setTls(v === true)}
        />
        {t('data.fields.tls')}
      </Label>
      {start.isError ? (
        <Alert variant="destructive" className="sm:col-span-2">
          <AlertTitle>{t('data.startError')}</AlertTitle>
          <AlertDescription>{start.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <div className="flex gap-2 sm:col-span-2">
        <Button type="submit" disabled={start.isPending}>
          {start.isPending ? t('data.starting') : t('data.start')}
        </Button>
        <Button type="button" variant="outline" onClick={onDone}>
          {t('data.cancel')}
        </Button>
      </div>
    </form>
  )
}

function CopyRow({ db }: { db: DatabaseCopy }) {
  const { t } = useTranslation('migration')
  const [open, setOpen] = useState(false)
  const bad = (db.tables ?? []).filter((x) => !x.ok)
  const canCopy = db.status !== 'unsupported' && db.status !== 'copying'
  return (
    <li className="rounded-md border p-3 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{db.database}</span>
        <span className="text-xs text-muted-foreground">{db.engine}</span>
        <Badge variant={statusVariant[db.status]}>
          {t(`data.status.${db.status}`)}
        </Badge>
        {db.checked > 0 ? (
          <span className="text-xs text-muted-foreground">
            {t('data.tablesChecked', {
              count: db.checked,
              bad: db.mismatched,
            })}
          </span>
        ) : null}
        {canCopy && !open ? (
          <Button
            size="sm"
            variant="outline"
            className="ml-auto"
            onClick={() => setOpen(true)}
          >
            {t('data.copy')}
          </Button>
        ) : null}
      </div>
      {db.reason ? <p className="mt-2 text-destructive">{db.reason}</p> : null}
      {bad.map((x) => (
        <p key={x.name} className="mt-1 font-mono text-xs">
          {t('data.differs', {
            name: x.name,
            source: x.source,
            target: x.target,
          })}
        </p>
      ))}
      {db.next_action && !open ? (
        <p className="mt-2 text-muted-foreground">{db.next_action}</p>
      ) : null}
      {open ? <CopyForm db={db} onDone={() => setOpen(false)} /> : null}
    </li>
  )
}

export function MigrationDataCard() {
  const { t } = useTranslation('migration')
  const { data, isError, isPending } = useDatabaseCopies()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('data.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">{t('data.description')}</p>
        {isError ? (
          <Alert variant="destructive">
            <AlertDescription>{t('data.loadError')}</AlertDescription>
          </Alert>
        ) : null}
        {!isPending && data?.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('data.empty')}</p>
        ) : null}
        <ul className="space-y-2">
          {data?.map((db) => (
            <CopyRow key={db.database} db={db} />
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
