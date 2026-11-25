import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import type { DatabaseCredential } from '../../types/databaseAccess'
import { useDatabase, useDatabaseStatus } from '../../queries/databases'
import { summarizeDatabaseStatus } from '../../lib/databaseStatus'
import { CredentialDialog } from './CredentialDialog'
import { TempAccessCard } from './TempAccessCard'
import { KV_ENGINES } from '../dbviewer/engines'
import { UsersCard } from './UsersCard'
import { WhoCanAccessCard } from './WhoCanAccessCard'

const USER_ENGINES = new Set(['postgres'])

// Which "no roles" copy applies, so each engine says what protects it instead.
function noRolesVariant(engine: string): 'keyvalue' | 'memcached' | 'other' {
  if (engine === 'memcached') return 'memcached'
  return KV_ENGINES.has(engine) ? 'keyvalue' : 'other'
}

/** AccessPanel is the database detail Access tab: logins, temporary credentials and platform access. */
export function AccessPanel({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('databaseAccess')
  const { t: tNav } = useTranslation('dbNav')
  const { data: database } = useDatabase(databaseName)
  const { data: conditions } = useDatabaseStatus(databaseName)
  const running = summarizeDatabaseStatus(conditions).variant !== 'destructive'
  const supported = USER_ENGINES.has(database.engine)
  const [credential, setCredential] = useState<DatabaseCredential | null>(null)

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
          {tNav('access.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {tNav('access.description')}{' '}
          <Link
            to="/databases/$name/credentials"
            params={{ name: databaseName }}
            className="underline underline-offset-2"
          >
            {tNav('access.credentialsChip')}
          </Link>
        </p>
      </div>
      {!supported ? (
        <Alert>
          <AlertTitle>
            {tNav(`access.${noRolesVariant(database.engine)}Title`)}
          </AlertTitle>
          <AlertDescription className="space-y-2">
            <p>{tNav(`access.${noRolesVariant(database.engine)}Body`)}</p>
            <Button
              variant="outline"
              size="sm"
              render={
                <Link
                  to="/databases/$name/networking"
                  params={{ name: databaseName }}
                />
              }
            >
              {tNav('access.openNetworking')}
            </Button>
          </AlertDescription>
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
