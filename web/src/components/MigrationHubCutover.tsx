import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  useHubConnection,
  useRevealHubConnection,
  type HubSession,
} from '../queries/migrationHub'
import { MigrationCutoverCard } from './MigrationCutoverCard'
import { MigrationVolumesCard } from './MigrationVolumesCard'

function Connection({
  id,
  db,
  target,
}: {
  id: string
  db: string
  target: string
}) {
  const { t } = useTranslation('migration')
  const base = useHubConnection(id, db, true)
  const reveal = useRevealHubConnection()
  const [shown, setShown] = useState(false)
  const conn = shown && reveal.data ? reveal.data : base.data

  return (
    <li className="rounded-md border p-3 text-sm">
      <p className="font-medium">
        {t('hub.cutover.connection', { name: target })}
      </p>
      {base.isError ? (
        <Alert variant="destructive" className="mt-2">
          <AlertDescription>{t('hub.cutover.loadError')}</AlertDescription>
        </Alert>
      ) : null}
      {conn ? (
        <>
          <p className="mt-1 text-muted-foreground">
            {t('hub.cutover.reference', { ref: conn.reference })}
          </p>
          <dl className="mt-2 grid gap-x-4 gap-y-1 font-mono text-xs sm:grid-cols-[10rem_1fr]">
            {conn.env.map((e) => (
              <div key={e.env_var} className="contents">
                <dt>{e.env_var}</dt>
                <dd className="break-all">{e.value}</dd>
              </div>
            ))}
          </dl>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            {shown ? (
              <Button
                size="sm"
                variant="outline"
                onClick={() => setShown(false)}
              >
                {t('hub.cutover.hide')}
              </Button>
            ) : (
              <Button
                size="sm"
                variant="outline"
                disabled={reveal.isPending}
                onClick={() =>
                  reveal.mutate({ id, db }, { onSuccess: () => setShown(true) })
                }
              >
                {reveal.isPending
                  ? t('hub.cutover.revealing')
                  : t('hub.cutover.reveal')}
              </Button>
            )}
            <span className="text-xs text-muted-foreground">
              {t('hub.cutover.revealNote')}
            </span>
          </div>
          {reveal.isError ? (
            <p className="mt-1 text-destructive">{reveal.error.message}</p>
          ) : null}
        </>
      ) : null}
    </li>
  )
}

export function MigrationHubCutover({ session }: { session: HubSession }) {
  const { t } = useTranslation('migration')
  const verified = session.items.filter((i) => i.status === 'verified')
  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">{t('hub.cutover.title')}</h3>
        <p className="text-sm text-muted-foreground">
          {t('hub.cutover.description')}
        </p>
      </div>
      <ul className="space-y-2">
        {verified.map((i) => (
          <Connection
            key={i.source_db}
            id={session.id}
            db={i.source_db}
            target={i.target_name}
          />
        ))}
      </ul>
      <MigrationVolumesCard />
      <MigrationCutoverCard />
    </div>
  )
}
