import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import {
  useAppImportSession,
  useAppImportSessions,
  useDeleteAppImportSession,
  usePutAppImportPlan,
  type AppImportSession,
} from '../queries/appImport'
import { AppImportConnect } from './AppImportConnect'
import { AppImportCutover } from './AppImportCutover'
import { AppImportImages } from './AppImportImages'
import { AppImportInventory } from './AppImportInventory'
import { AppImportPreflight } from './AppImportPreflight'
import { AppImportRun } from './AppImportRun'
import { AppImportVolumes } from './AppImportVolumes'
import {
  APP_IMPORT_STEPS,
  stepReached,
  type AppImportViewStep,
} from './appImportFormat'

function Stepper({
  current,
  reached,
  onGo,
}: {
  current: AppImportViewStep
  reached: number
  onGo: (s: AppImportViewStep) => void
}) {
  const { t } = useTranslation('migration')
  return (
    <ol className="flex flex-wrap gap-2" aria-label={t('appImport.title')}>
      {APP_IMPORT_STEPS.map((s, i) => (
        <li key={s}>
          <Button
            size="sm"
            variant={s === current ? 'default' : 'outline'}
            disabled={i > reached}
            aria-current={s === current ? 'step' : undefined}
            onClick={() => onGo(s)}
          >
            {i + 1}. {t(`appImport.steps.${s}`)}
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
      <p className="font-medium">{t('appImport.safe.title')}</p>
      <ul className="mt-1 list-disc space-y-0.5 pl-5 text-muted-foreground">
        <li>{t('appImport.safe.readOnly')}</li>
        <li>{t('appImport.safe.staged')}</li>
        <li>{t('appImport.safe.volumes')}</li>
        <li>{t('appImport.safe.dns')}</li>
      </ul>
    </div>
  )
}

function Session({
  session,
  onForget,
}: {
  session: AppImportSession
  onForget: () => void
}) {
  const { t } = useTranslation('migration')
  const put = usePutAppImportPlan()
  const reached = stepReached(session.step)
  const [view, setView] = useState<AppImportViewStep | null>(null)
  const current: AppImportViewStep =
    view ?? APP_IMPORT_STEPS[reached] ?? 'inventory'

  function go(s: AppImportViewStep) {
    setView(s)
    const idx = APP_IMPORT_STEPS.indexOf(s)
    if (s !== 'connect' && idx > reached) {
      put.mutate({ id: session.id, update: { step: s } })
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-sm text-muted-foreground">
          {t('appImport.resume', { url: session.source_url })}
        </p>
        <Button
          size="sm"
          variant="ghost"
          className="ml-auto"
          title={t('appImport.forgetHelp')}
          disabled={session.running}
          onClick={onForget}
        >
          {t('appImport.forget')}
        </Button>
      </div>
      <Stepper current={current} reached={reached} onGo={setView} />
      {current === 'connect' ? (
        <AppImportConnect
          reconnect={{
            id: session.id,
            url: session.source_url,
            platform: session.platform,
          }}
          onCreated={() => go('inventory')}
        />
      ) : null}
      {current === 'inventory' ? (
        <AppImportInventory session={session} onNext={() => go('preflight')} />
      ) : null}
      {current === 'preflight' ? (
        <AppImportPreflight
          session={session}
          onBack={() => go('inventory')}
          onNext={() => go('stage')}
        />
      ) : null}
      {current === 'stage' ? (
        <AppImportRun
          session={session}
          verifyView={false}
          onBack={() => go('preflight')}
          onNext={() => go('images')}
        />
      ) : null}
      {current === 'images' ? (
        <AppImportImages
          session={session}
          onBack={() => go('stage')}
          onNext={() => go('verify')}
        />
      ) : null}
      {current === 'verify' ? (
        <AppImportRun
          session={session}
          verifyView
          onBack={() => go('images')}
          onNext={() => go('volumes')}
        />
      ) : null}
      {current === 'volumes' ? (
        <AppImportVolumes
          session={session}
          onBack={() => go('verify')}
          onNext={() => go('cutover')}
        />
      ) : null}
      {current === 'cutover' ? (
        <AppImportCutover session={session} onBack={() => go('volumes')} />
      ) : null}
    </div>
  )
}

export function AppImportWizard() {
  const { t } = useTranslation('migration')
  const list = useAppImportSessions()
  const [picked, setPicked] = useState<string | null>(null)
  const [fresh, setFresh] = useState(false)
  const del = useDeleteAppImportSession()
  const latest = list.data?.[0]?.id ?? null
  const id = fresh ? picked : (picked ?? latest)
  const session = useAppImportSession(id)

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
        <CardTitle>{t('appImport.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          {t('appImport.description')}
        </p>
        <SafeToSkip />
        {list.isError || session.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{t('appImport.loadError')}</AlertDescription>
          </Alert>
        ) : null}
        {session.data ? (
          <Session
            key={session.data.id}
            session={session.data}
            onForget={forget}
          />
        ) : list.isPending ? null : (
          <AppImportConnect
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
            {t('appImport.newImport')}
          </Button>
        ) : null}
      </CardContent>
    </Card>
  )
}
