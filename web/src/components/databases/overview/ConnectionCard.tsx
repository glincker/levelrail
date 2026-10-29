import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { EyeGlassIcon, EyeOffGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { defaultConnectUser } from '@/lib/databaseConnect'
import { buildSnippets, type SnippetTab } from '@/lib/databaseSnippets'
import { useDatabaseNetwork } from '../../../queries/databaseAccess'
import type { DatabaseResource } from '../../../types/databaseDetail'
import { CopyField } from '../../traffic/CopyField'

const TABS: SnippetTab[] = ['uri', 'env', 'node', 'python', 'go', 'cli']

export function ConnectionCard({ database }: { database: DatabaseResource }) {
  const { t } = useTranslation('databases')
  const [reveal, setReveal] = useState(false)
  const network = useDatabaseNetwork(database.name)
  const engine = database.engine
  const tls = network.data?.tls.enabled ?? database.tls_enabled ?? false

  const snippets = network.data
    ? buildSnippets({
        engine,
        host: network.data.internal.host,
        port: network.data.internal.port,
        user: defaultConnectUser(engine),
        database: engine === 'postgres' ? 'postgres' : '',
        tls,
        revealPlaceholder: reveal,
      })
    : null
  const meshUri = network.data?.mesh
    ? buildSnippets({
        engine,
        host: network.data.mesh.host,
        port: network.data.mesh.port,
        user: defaultConnectUser(engine),
        database: engine === 'postgres' ? 'postgres' : '',
        tls,
        revealPlaceholder: reveal,
      }).uri
    : ''
  const tabs = TABS.filter((id) => (snippets ? snippets[id] !== '' : true))

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle>{t('connection.title')}</CardTitle>
          <div className="flex items-center gap-2">
            <Badge variant={tls ? 'success' : 'muted'}>
              {tls ? t('connection.tlsOn') : t('connection.tlsOff')}
            </Badge>
            <Button
              size="sm"
              variant="ghost"
              aria-pressed={reveal}
              onClick={() => setReveal((v) => !v)}
            >
              {reveal ? (
                <EyeOffGlassIcon size="1.2em" aria-hidden="true" />
              ) : (
                <EyeGlassIcon size="1.2em" aria-hidden="true" />
              )}
              {reveal
                ? t('connection.maskPassword')
                : t('connection.showPlaceholder')}
            </Button>
          </div>
        </div>
        <p className="text-sm text-muted-foreground">
          {t('connection.passwordNever')}
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        {network.isPending ? (
          <div className="space-y-2" aria-hidden="true">
            <Skeleton className="h-8 w-64" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : !snippets ? (
          <div className="space-y-2">
            <p className="text-sm text-muted-foreground">
              {t('connection.unavailable')}
            </p>
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                void network.refetch()
              }}
            >
              {t('connection.retry')}
            </Button>
          </div>
        ) : (
          <>
            {meshUri ? (
              <div className="space-y-1">
                <p className="text-sm font-medium">{t('connection.forApps')}</p>
                <CopyField value={meshUri} label={t('connection.forApps')} />
                <p className="text-xs text-muted-foreground">
                  {t('connection.forAppsHint')}
                </p>
              </div>
            ) : null}
            <Tabs defaultValue="uri">
              <TabsList className="max-w-full">
                {tabs.map((id) => (
                  <TabsTrigger key={id} value={id}>
                    {t(`connection.tab.${id}`, { engine })}
                  </TabsTrigger>
                ))}
              </TabsList>
              {tabs.map((id) => (
                <TabsContent key={id} value={id} className="pt-2">
                  <CopyField
                    value={snippets[id]}
                    label={t(`connection.tab.${id}`, { engine })}
                    multiline={id === 'node' || id === 'python' || id === 'go'}
                  />
                </TabsContent>
              ))}
            </Tabs>
          </>
        )}
        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
          <p className="min-w-0 text-sm text-muted-foreground">
            {t('connection.credentialsHint')}
          </p>
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
