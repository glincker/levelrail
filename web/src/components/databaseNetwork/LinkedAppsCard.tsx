import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { NativeSelect } from '@/components/ui/native-select'
import { toast } from '@/components/ui/toast'
import { useAppListOptional } from '../../queries/apps'
import {
  useConnectDatabaseApp,
  useDatabaseLinks,
} from '../../queries/databaseLinks'

/** LinkedAppsCard answers "who connects": the apps linked to this database and the env var each one gets. */
export function LinkedAppsCard({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('dbNav')
  const { t: tDb } = useTranslation('databases')
  const [selected, setSelected] = useState('')
  const links = useDatabaseLinks(databaseName)
  const apps = useAppListOptional()
  const connect = useConnectDatabaseApp(databaseName)
  const linked = new Set((links.data ?? []).map((l) => l.app))
  const candidates = (apps.data ?? []).filter((a) => !linked.has(a.name))

  function link() {
    if (!selected) return
    connect.mutate(
      { app: selected },
      {
        onSuccess: () => {
          setSelected('')
          toast.add({ title: tDb('links.toastConnected'), type: 'success' })
        },
        onError: (err) =>
          toast.add({
            title: tDb('links.toastFailed'),
            description: err.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-1.5 text-sm">
          <PlugGlassIcon className="size-5" aria-hidden="true" />
          {t('networking.linkedTitle')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {links.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{tDb('links.loadFailed')}</AlertDescription>
          </Alert>
        ) : links.data && links.data.length === 0 ? (
          <p className="text-sm text-muted-foreground">{tDb('links.empty')}</p>
        ) : (
          <ul className="space-y-1.5">
            {(links.data ?? []).map((l) => (
              <li
                key={l.app}
                className="flex flex-wrap items-center gap-x-2 text-sm"
              >
                <Link
                  to="/apps/$name"
                  params={{ name: l.app }}
                  className="font-medium hover:underline"
                >
                  {l.app}
                </Link>
                <span className="text-xs text-muted-foreground">
                  {tDb('links.envVar')}:{' '}
                  <code className="font-mono">{l.env_var}</code>
                </span>
              </li>
            ))}
          </ul>
        )}
        {candidates.length > 0 ? (
          <div className="flex items-center gap-2">
            <NativeSelect
              aria-label={tDb('links.appLabel')}
              className="min-w-0 flex-1"
              value={selected}
              onChange={(e) => setSelected(e.target.value)}
            >
              <option value="">{tDb('links.appPlaceholder')}</option>
              {candidates.map((a) => (
                <option key={a.name} value={a.name}>
                  {a.name}
                </option>
              ))}
            </NativeSelect>
            <Button
              size="sm"
              writeAction
              disabled={!selected || connect.isPending}
              onClick={link}
            >
              {t('networking.linkApp')}
            </Button>
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">{tDb('links.noApps')}</p>
        )}
      </CardContent>
    </Card>
  )
}
