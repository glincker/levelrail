import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FadersIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import type {
  DatabaseNetwork,
  NetworkScope,
  ScopeResult,
} from '../../types/databaseAccess'
import { useSetScope } from '../../queries/databaseAccess'

const SCOPES: NetworkScope[] = ['platform', 'project', 'environment']

/** ScopeCard limits which apps the platform attaches to this database, always showing the impact before changing anything. */
export function ScopeCard({
  databaseName,
  network,
}: {
  databaseName: string
  network: DatabaseNetwork
}) {
  const { t } = useTranslation('databaseAccess')
  const setScope = useSetScope(databaseName)
  const [scope, setScope_] = useState<NetworkScope>(network.scope.current)
  const [result, setResult] = useState<ScopeResult | null>(null)

  const changed = scope !== network.scope.current
  const lost = result?.lost ?? []

  function fail(err: Error) {
    toast.add({
      title: t('scope.failedToast'),
      description: err.message,
      type: 'error',
    })
  }

  function check() {
    setScope.mutate(
      { scope, dry_run: true },
      { onSuccess: setResult, onError: fail },
    )
  }

  function apply() {
    setScope.mutate(
      { scope, confirm: true },
      {
        onSuccess: (res) => {
          if (res.applied) {
            toast.add({ title: t('scope.appliedToast'), type: 'success' })
            setResult(null)
          }
        },
        onError: fail,
      },
    )
  }

  return (
    <Card>
      <CardHeader className="space-y-1">
        <CardTitle className="flex items-center gap-1.5 text-sm">
          <FadersIcon className="size-4" aria-hidden="true" />
          {t('scope.title')}
        </CardTitle>
        <p className="text-xs text-muted-foreground">
          {t('scope.description')}
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-xs text-muted-foreground">
          {t('scope.placement', {
            project:
              network.scope.project_name ||
              network.scope.project_id ||
              t('scope.unplaced'),
            environment: network.scope.environment || t('scope.unplaced'),
          })}
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <Select
            value={scope}
            onValueChange={(v) => {
              setScope_(v as NetworkScope)
              setResult(null)
            }}
          >
            <SelectTrigger className="w-64" aria-label={t('scope.title')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {SCOPES.map((s) => (
                <SelectItem key={s} value={s}>
                  {t(`scope.options.${s}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            variant="outline"
            disabled={!changed || setScope.isPending}
            onClick={check}
          >
            {t('scope.check')}
          </Button>
          <Button disabled={!result || setScope.isPending} onClick={apply}>
            {lost.length > 0
              ? t('scope.applyAnyway', { count: lost.length })
              : t('scope.apply')}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          {t(`scope.hint.${scope}`)}
        </p>
        {result ? (
          <Alert variant={lost.length > 0 ? 'destructive' : 'default'}>
            <AlertTitle>{t('scope.dryRunTitle')}</AlertTitle>
            <AlertDescription className="space-y-1">
              <span>
                {lost.length === 0
                  ? t('scope.noneLost')
                  : t('scope.lost', { count: lost.length })}
              </span>
              {lost.length > 0 ? (
                <ul className="list-disc pl-4">
                  {lost.map((v) => (
                    <li key={v.app.name}>{v.reason ?? v.app.name}</li>
                  ))}
                </ul>
              ) : null}
            </AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
    </Card>
  )
}
