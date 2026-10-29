import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useAppListOptional } from '../../queries/apps'
import {
  useConnectDatabaseApp,
  useDatabaseLinks,
} from '../../queries/databaseLinks'

export function DatabaseLinksCard({ database }: { database: string }) {
  const { t } = useTranslation('databases')
  const [selected, setSelected] = useState('')
  const links = useDatabaseLinks(database)
  const apps = useAppListOptional()
  const connect = useConnectDatabaseApp(database)

  const linked = new Set((links.data ?? []).map((l) => l.app))
  const candidates = (apps.data ?? []).filter((a) => !linked.has(a.name))

  function handleConnect() {
    if (!selected) return
    const app = selected
    connect.mutate(
      { app },
      {
        onSuccess: (link) => {
          setSelected('')
          toast.add({
            title: t('links.toastConnected'),
            description: t('links.toastConnectedBody', {
              app,
              envVar: link.env_var,
            }),
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({
            title: t('links.toastFailed'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('links.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          {t('links.description')}
        </p>
        {links.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{t('links.loadFailed')}</AlertDescription>
          </Alert>
        ) : links.data && links.data.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('links.empty')}</p>
        ) : (
          <ul className="space-y-1.5">
            {(links.data ?? []).map((l) => (
              <li key={l.app} className="flex items-center gap-2 text-sm">
                <PlugGlassIcon
                  className="size-5 shrink-0 text-muted-foreground"
                  aria-hidden="true"
                />
                <Link
                  to="/apps/$name"
                  params={{ name: l.app }}
                  className="font-medium hover:underline"
                >
                  {l.app}
                </Link>
                <span className="text-xs text-muted-foreground">
                  {t('links.envVar')}:{' '}
                  <code className="font-mono">{l.env_var}</code>
                </span>
              </li>
            ))}
          </ul>
        )}
        {candidates.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('links.noApps')}</p>
        ) : (
          <div className="flex items-center gap-2">
            <Select
              value={selected}
              onValueChange={(value) => {
                if (value) setSelected(value)
              }}
            >
              <SelectTrigger
                aria-label={t('links.appLabel')}
                className="w-full"
              >
                <SelectValue placeholder={t('links.appPlaceholder')} />
              </SelectTrigger>
              <SelectContent>
                {candidates.map((a) => (
                  <SelectItem key={a.name} value={a.name}>
                    {a.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              type="button"
              size="sm"
              writeAction
              disabled={connect.isPending || !selected}
              onClick={handleConnect}
            >
              {connect.isPending ? t('links.connecting') : t('links.connect')}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
