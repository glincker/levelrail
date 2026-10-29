import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { PinGlassIcon, RotateCcwGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { useRollbackToDeploy } from '../../queries/deployControl'
import {
  functionControlKeys,
  useFunctionVersions,
  type FunctionVersion,
} from '../../queries/functionControls'
import { usePublishLockActions } from '../../queries/publishLock'
import { RelativeTime } from '../kit'

function versionLabel(v: FunctionVersion): string {
  return v.commit_sha ? v.commit_sha.slice(0, 7) : v.image
}

export function FunctionVersionsTab({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('functions')
  const qc = useQueryClient()
  const { data: versions, isPending } = useFunctionVersions(name)
  const rollback = useRollbackToDeploy(name)
  const { rollbackAndLock } = usePublishLockActions(name)
  const busy = rollback.isPending || rollbackAndLock.isPending

  const done = () => {
    void qc.invalidateQueries({ queryKey: functionControlKeys.versions(name) })
    toast.add({ title: t('detail.versions.done'), type: 'success' })
  }
  const failed = (e: Error) =>
    toast.add({
      title: t('detail.versions.failed'),
      description: e.message,
      type: 'error',
    })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.versions.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {t('detail.versions.help')}
        </p>
        {isPending ? (
          <div className="h-16 animate-pulse rounded-md bg-muted" />
        ) : null}
        {versions?.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('detail.versions.empty')}
          </p>
        ) : null}
        <ul className="divide-y divide-border">
          {versions?.map((v) => (
            <li
              key={v.deploy_id}
              className="flex flex-wrap items-center justify-between gap-3 py-2"
            >
              <div className="min-w-0 space-y-0.5">
                <p className="flex items-center gap-2 text-sm font-medium">
                  <span className="truncate">{versionLabel(v)}</span>
                  {v.live ? <Badge>{t('detail.versions.live')}</Badge> : null}
                  {v.status === 'succeeded' ? null : (
                    <Badge variant="muted">{v.status}</Badge>
                  )}
                </p>
                <p className="text-xs text-muted-foreground">
                  <RelativeTime at={v.started_at} />
                </p>
              </div>
              {v.rollbackable ? (
                <div className="flex gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={() =>
                      rollback.mutate(
                        { deployId: v.deploy_id, confirm: true },
                        { onSuccess: done, onError: failed },
                      )
                    }
                  >
                    <RotateCcwGlassIcon size="1.4em" aria-hidden />
                    {t('detail.versions.rollback')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={() =>
                      rollbackAndLock.mutate(v.deploy_id, {
                        onSuccess: done,
                        onError: failed,
                      })
                    }
                  >
                    <PinGlassIcon size="1.4em" aria-hidden />
                    {t('detail.versions.pin')}
                  </Button>
                </div>
              ) : null}
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
