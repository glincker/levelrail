import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import {
  useDeleteHubSession,
  useHubSession,
  useHubSessions,
  type HubSession,
} from '../queries/migrationHub'
import { MigrationDataCard } from './MigrationDataCard'
import { MigrationHubConnect } from './MigrationHubConnect'
import { MigrationHubCopy } from './MigrationHubCopy'
import { MigrationHubCutover } from './MigrationHubCutover'
import { MigrationHubInventory } from './MigrationHubInventory'
import { MigrationHubPreflight } from './MigrationHubPreflight'
import { HUB_STEPS, type HubViewStep } from './migrationHubFormat'

function Stepper({
  current,
  reached,
  onGo,
}: {
  current: HubViewStep
  reached: number
  onGo: (s: HubViewStep) => void
}) {
  const { t } = useTranslation('migration')
  return (
    <ol className="flex flex-wrap gap-2" aria-label={t('hub.title')}>
      {HUB_STEPS.map((s, i) => (
        <li key={s}>
          <Button
            size="sm"
            variant={s === current ? 'default' : 'outline'}
            disabled={i > reached}
            aria-current={s === current ? 'step' : undefined}
            onClick={() => onGo(s)}
          >
            {i + 1}. {t(`hub.steps.${s}`)}
          </Button>
        </li>
      ))}
    </ol>
  )
}

function SafeToSkip() {
  const { t } = useTranslation('migration')
  return (
    <div className="rounded-md border bg-muted/30 p-3 text-sm">
      <p className="font-medium">{t('hub.safeToSkip.title')}</p>
      <ul className="mt-1 list-disc space-y-0.5 pl-5 text-muted-foreground">
        <li>{t('hub.safeToSkip.never')}</li>
        <li>{t('hub.safeToSkip.warnings')}</li>
        <li>{t('hub.safeToSkip.volumes')}</li>
        <li>{t('hub.safeToSkip.cutover')}</li>
      </ul>
    </div>
  )
}

function stepIndex(step: string): number {
  const i = HUB_STEPS.indexOf(step as HubViewStep)
  return i < 0 ? 1 : i
}

function Session({
  session,
  onForget,
  onCreated,
}: {
  session: HubSession
  onForget: () => void
  onCreated: (id: string) => void
}) {
  const { t } = useTranslation('migration')
  const reached = stepIndex(session.step)
  const [view, setView] = useState<HubViewStep | null>(null)
  const current: HubViewStep = view ?? HUB_STEPS[reached] ?? 'inventory'
  const go = (s: HubViewStep) => setView(s)

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-sm text-muted-foreground">
          {t('hub.resume', { host: session.host })}
        </p>
        <Button
          size="sm"
          variant="ghost"
          className="ml-auto"
          title={t('hub.forgetHelp')}
          disabled={session.running}
          onClick={onForget}
        >
          {t('hub.forget')}
        </Button>
      </div>
      <Stepper current={current} reached={reached} onGo={go} />
      {current === 'connect' ? (
        <MigrationHubConnect onCreated={onCreated} />
      ) : null}
      {current === 'inventory' ? (
        <MigrationHubInventory
          session={session}
          onNext={() => go('preflight')}
        />
      ) : null}
      {current === 'preflight' ? (
        <MigrationHubPreflight
          session={session}
          onBack={() => go('inventory')}
          onStarted={() => go('copy')}
        />
      ) : null}
      {current === 'copy' ? (
        <MigrationHubCopy
          session={session}
          verifyView={false}
          onNext={() => go('verify')}
        />
      ) : null}
      {current === 'verify' ? (
        <MigrationHubCopy
          session={session}
          verifyView
          onNext={() => go('cutover')}
        />
      ) : null}
      {current === 'cutover' ? <MigrationHubCutover session={session} /> : null}
    </div>
  )
}

export function MigrationHub() {
  const { t } = useTranslation('migration')
  const list = useHubSessions()
  const [picked, setPicked] = useState<string | null>(null)
  const [fresh, setFresh] = useState(false)
  const del = useDeleteHubSession()
  const latest = list.data?.[0]?.id ?? null
  const id = fresh ? picked : (picked ?? latest)
  const session = useHubSession(id)

  function forget() {
    if (!id) return
    del.mutate(id, {
      onSuccess: () => {
        setPicked(null)
        setFresh(true)
      },
      onError: (e) => toast.add({ title: e.message, type: 'error' }),
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('hub.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">{t('hub.description')}</p>
        <SafeToSkip />
        {list.isError || session.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{t('hub.loadError')}</AlertDescription>
          </Alert>
        ) : null}
        {session.data ? (
          <Session
            key={session.data.id}
            session={session.data}
            onForget={forget}
            onCreated={(newId) => {
              setPicked(newId)
              setFresh(false)
            }}
          />
        ) : list.isPending ? null : (
          <MigrationHubConnect
            onCreated={(newId) => {
              setPicked(newId)
              setFresh(false)
            }}
          />
        )}
        {session.data ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setPicked(null)
              setFresh(true)
            }}
          >
            {t('hub.newMigration')}
          </Button>
        ) : (
          <details>
            <summary className="cursor-pointer text-sm text-muted-foreground">
              {t('hub.connect.advanced')}
            </summary>
            <div className="mt-3">
              <MigrationDataCard />
            </div>
          </details>
        )}
      </CardContent>
    </Card>
  )
}
