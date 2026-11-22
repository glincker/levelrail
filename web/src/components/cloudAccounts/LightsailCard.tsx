import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { messageOf } from '../../lib/cloudAwsServices'
import { lightsailCanChange } from '../../lib/cloudLightsail'
import {
  useAwsLightsailAction,
  useAwsLightsailInstances,
} from '../../queries/cloudLightsail'
import { StateLine } from './AwsServiceShared'

export function LightsailCard({
  id,
  canDelete,
}: Readonly<{ id: string; canDelete: boolean }>) {
  const { t } = useTranslation('cloudAccounts')
  const list = useAwsLightsailInstances(id)
  const act = useAwsLightsailAction(id)
  const [deleting, setDeleting] = useState<string | null>(null)
  const empty =
    list.data?.length === 0 ? t('services.lightsail.empty') : undefined
  return (
    <div className="space-y-2">
      {act.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(act.error)}
        </p>
      ) : null}
      <StateLine pending={list.isPending} error={list.error} empty={empty} />
      <ul className="space-y-1">
        {(list.data ?? []).map((i) => (
          <li
            key={i.name}
            className="flex flex-wrap items-center gap-2 text-sm"
          >
            <span className="font-mono text-xs">{i.name}</span>
            <span className="text-xs text-muted-foreground">
              {i.state} · {i.bundle_id}
              {i.public_ip ? ` · ${i.public_ip}` : ''}
            </span>
            {lightsailCanChange(i) ? (
              <span className="ml-auto flex gap-1">
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    act.mutate({
                      kind: i.state === 'running' ? 'stop' : 'start',
                      name: i.name,
                    })
                  }}
                >
                  {i.state === 'running'
                    ? t('services.lightsail.stop')
                    : t('services.lightsail.start')}
                </Button>
                {i.state === 'running' ? (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      act.mutate({ kind: 'reboot', name: i.name })
                    }}
                  >
                    {t('services.lightsail.reboot')}
                  </Button>
                ) : null}
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    act.mutate({ kind: 'snapshot', name: i.name })
                  }}
                >
                  {t('services.lightsail.snapshot')}
                </Button>
                {canDelete ? (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setDeleting(i.name)
                    }}
                  >
                    {t('services.lightsail.delete')}
                  </Button>
                ) : null}
              </span>
            ) : null}
          </li>
        ))}
      </ul>
      {deleting === null ? null : (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setDeleting(null)
          }}
          title={t('services.lightsail.deleteTitle')}
          description={t('services.lightsail.deleteBody')}
          confirmLabel={t('services.lightsail.delete')}
          requireTyped={deleting}
          pending={act.isPending}
          error={act.isError ? messageOf(act.error) : null}
          onConfirm={() => {
            act.mutate(
              { kind: 'delete', name: deleting },
              {
                onSuccess: () => {
                  setDeleting(null)
                },
              },
            )
          }}
        />
      )}
    </div>
  )
}
