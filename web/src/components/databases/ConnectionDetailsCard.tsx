import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { buttonVariants } from '@/components/ui/button'
import { CopyField } from '../traffic/CopyField'
import { flavorLabel, flavorOfVersion } from '@/lib/databaseCatalog'
import type { DatabaseResource } from '../../types/databaseDetail'

function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0 space-y-1">
      <dt className="text-xs text-muted-foreground uppercase">{label}</dt>
      <dd className="text-sm text-foreground">{children}</dd>
    </div>
  )
}

// What the dashboard can truthfully show about reaching a managed database.
// Passwords are never returned by the API, so credentials live on Access.
export function ConnectionDetailsCard({
  database,
}: {
  database: DatabaseResource
}) {
  const { t } = useTranslation('databases')
  const flavor =
    database.engine === 'postgres' ? flavorOfVersion(database.version) : null
  const publicAddress =
    database.publicly_accessible && database.public_port
      ? `${window.location.hostname}:${database.public_port}`
      : null

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('connection.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('connection.description')}
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Detail label={t('connection.engine')}>
            <span className="capitalize">{database.engine}</span>{' '}
            <span className="font-mono text-muted-foreground">
              {database.version}
            </span>
            {flavor ? (
              <Badge variant="muted" className="ml-1">
                {t('picker.flavorBadge', { flavor: flavorLabel(flavor) })}
              </Badge>
            ) : null}
          </Detail>
          <Detail label={t('connection.tls')}>
            {database.tls_enabled ? (
              <Badge variant="success">{t('connection.tlsOn')}</Badge>
            ) : (
              <Badge variant="muted">{t('connection.tlsOff')}</Badge>
            )}
          </Detail>
        </dl>
        <div className="space-y-1.5">
          <p className="text-xs text-muted-foreground uppercase">
            {t('connection.address')}
          </p>
          {publicAddress ? (
            <CopyField
              value={publicAddress}
              label={t('connection.copyAddress')}
            />
          ) : (
            <>
              <p className="text-sm font-medium text-foreground">
                {t('connection.addressPrivate')}
              </p>
              <p className="text-sm text-muted-foreground">
                {t('connection.addressPrivateHint')}
              </p>
            </>
          )}
        </div>
        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
          <div className="min-w-0">
            <p className="text-sm font-medium text-foreground">
              {t('connection.credentials')}
            </p>
            <p className="text-sm text-muted-foreground">
              {t('connection.credentialsHint')}
            </p>
          </div>
          <Link
            to="/databases/$name/access"
            params={{ name: database.name }}
            className={buttonVariants({ variant: 'outline', size: 'sm' })}
          >
            {t('connection.openAccess')}
          </Link>
        </div>
      </CardContent>
    </Card>
  )
}
