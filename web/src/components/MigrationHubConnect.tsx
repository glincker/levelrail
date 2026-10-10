import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  useCreateHubSession,
  useHubLocalSources,
} from '../queries/migrationHub'

const ENGINES = ['postgres', 'mysql', 'mariadb', 'mongodb'] as const

export function MigrationHubConnect({
  onCreated,
}: {
  onCreated: (id: string) => void
}) {
  const { t } = useTranslation('migration')
  const create = useCreateHubSession()
  const [engine, setEngine] = useState<string>('postgres')
  const [host, setHost] = useState('')
  const [port, setPort] = useState('')
  const [user, setUser] = useState('')
  const [password, setPassword] = useState('')
  const [tls, setTls] = useState(false)
  const [container, setContainer] = useState('')
  const locals = useHubLocalSources()
  const isLocal = container !== ''

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    create.mutate(
      {
        engine,
        container: container || undefined,
        host: isLocal ? '' : host.trim(),
        port: port && !isLocal ? Number(port) : undefined,
        user: user || undefined,
        password: password || undefined,
        tls: tls || undefined,
      },
      {
        onSuccess: (s) => {
          setPassword('')
          onCreated(s.id)
        },
      },
    )
  }

  return (
    <form onSubmit={onSubmit} className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">{t('hub.connect.title')}</h3>
        <p className="text-sm text-muted-foreground">
          {t('hub.connect.description')}
        </p>
      </div>
      {locals.data && locals.data.length > 0 ? (
        <div className="space-y-1.5">
          <Label htmlFor="hub-container">{t('hub.connect.local')}</Label>
          <select
            id="hub-container"
            value={container}
            onChange={(e) => {
              setContainer(e.target.value)
              const picked = locals.data?.find(
                (l) => l.container === e.target.value,
              )
              if (picked) setEngine(picked.engine)
            }}
            className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
          >
            <option value="">{t('hub.connect.localNone')}</option>
            {locals.data.map((l) => (
              <option
                key={l.container}
                value={l.container}
                disabled={!!l.problem}
              >
                {l.problem
                  ? t('hub.connect.localProblem', {
                      name: l.container,
                      problem: l.problem,
                    })
                  : `${l.container} (${l.engine})`}
              </option>
            ))}
          </select>
          <p className="text-xs text-muted-foreground">
            {t('hub.connect.localHelp')}
          </p>
        </div>
      ) : null}
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="hub-engine">{t('hub.connect.engine')}</Label>
          <select
            id="hub-engine"
            disabled={isLocal}
            value={engine}
            onChange={(e) => setEngine(e.target.value)}
            className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
          >
            {ENGINES.map((e) => (
              <option key={e} value={e}>
                {e}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="hub-host">{t('hub.connect.host')}</Label>
          <Input
            id="hub-host"
            required={!isLocal}
            disabled={isLocal}
            value={host}
            onChange={(e) => setHost(e.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="hub-port">{t('hub.connect.port')}</Label>
          <Input
            id="hub-port"
            disabled={isLocal}
            type="number"
            min={1}
            max={65535}
            value={port}
            onChange={(e) => setPort(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            {t('hub.connect.portHelp')}
          </p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="hub-user">{t('hub.connect.user')}</Label>
          <Input
            id="hub-user"
            autoComplete="off"
            value={user}
            onChange={(e) => setUser(e.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="hub-password">{t('hub.connect.password')}</Label>
          <Input
            id="hub-password"
            type="password"
            autoComplete="off"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        <Label className="flex items-center gap-2 self-end pb-2 font-normal">
          <Checkbox
            aria-label={t('hub.connect.tls')}
            checked={tls}
            onCheckedChange={(v) => setTls(v === true)}
          />
          {t('hub.connect.tls')}
        </Label>
      </div>
      <p className="text-xs text-muted-foreground">
        {t('hub.connect.readOnly')}
      </p>
      {create.isError ? (
        <Alert variant="destructive">
          <AlertTitle>{t('hub.connect.error')}</AlertTitle>
          <AlertDescription className="break-words whitespace-pre-wrap">
            {create.error.message}
          </AlertDescription>
        </Alert>
      ) : null}
      <Button type="submit" disabled={create.isPending}>
        {create.isPending
          ? t('hub.connect.submitting')
          : t('hub.connect.submit')}
      </Button>
    </form>
  )
}
