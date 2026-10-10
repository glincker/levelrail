import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import type { DatabaseCredential } from '../../types/databaseAccess'
import { useDatabase, useDatabaseStatus } from '../../queries/databases'
import { summarizeDatabaseStatus } from '../../lib/databaseStatus'
import { CredentialDialog } from './CredentialDialog'
import { TempAccessCard } from './TempAccessCard'
import { UsersCard } from './UsersCard'
import { WhoCanAccessCard } from './WhoCanAccessCard'

const USER_ENGINES = new Set(['postgres'])

/** AccessPanel is the database detail Access tab: logins, temporary credentials and platform access. */
export function AccessPanel({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('databaseAccess')
  const { data: database } = useDatabase(databaseName)
  const { data: conditions } = useDatabaseStatus(databaseName)
  const running = summarizeDatabaseStatus(conditions).variant !== 'destructive'
  const supported = USER_ENGINES.has(database.engine)
  const [credential, setCredential] = useState<DatabaseCredential | null>(null)

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-base font-semibold">{t('access.title')}</h2>
        <p className="text-sm text-muted-foreground">
          {t('access.description')}
        </p>
      </div>
      {!supported ? (
        <Alert>
          <AlertTitle>{t('access.unsupportedTitle')}</AlertTitle>
          <AlertDescription>{t('access.unsupportedBody')}</AlertDescription>
        </Alert>
      ) : !running ? (
        <Alert>
          <AlertTitle>{t('access.notRunningTitle')}</AlertTitle>
          <AlertDescription>{t('access.notRunningBody')}</AlertDescription>
        </Alert>
      ) : (
        <>
          <TempAccessCard
            databaseName={databaseName}
            onCredential={setCredential}
          />
          <UsersCard databaseName={databaseName} onCredential={setCredential} />
        </>
      )}
      <WhoCanAccessCard databaseName={databaseName} />
      <CredentialDialog
        credential={credential}
        onClose={() => setCredential(null)}
      />
    </div>
  )
}
