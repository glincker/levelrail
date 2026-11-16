import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronRightGlassIcon,
  PlugZapGlassIcon,
  TrashGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { RoleGate } from '@/components/ui/RoleGate'
import { toast } from '@/components/ui/toast'
import { useDisconnectCluster, useTestCluster } from '../../queries/clusters'
import type { Cluster } from '../../types/clusters'
import { ClusterDetail } from './ClusterDetail'

const KNOWN_CLASSES = [
  'ok',
  'unauthorized',
  'forbidden',
  'error',
  'unreachable',
] as const
type KnownClass = (typeof KNOWN_CLASSES)[number]

function knownClass(c: string): KnownClass | null {
  return KNOWN_CLASSES.find((k) => k === c) ?? null
}

export function ClusterRow({ cluster }: Readonly<{ cluster: Cluster }>) {
  const { t } = useTranslation('clusters')
  const [open, setOpen] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const test = useTestCluster()
  const disconnect = useDisconnectCluster()

  const checkClass = test.data?.class ?? cluster.last_check_class ?? ''
  const known = knownClass(checkClass)
  const statusKey = known ?? 'untested'

  return (
    <li>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3 py-2 text-sm">
        <button
          type="button"
          aria-expanded={open}
          aria-label={t('list.toggle', { name: cluster.name })}
          onClick={() => {
            setOpen((v) => !v)
          }}
          className="flex min-w-0 flex-1 basis-56 items-center gap-2 rounded text-left focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          <ChevronRightGlassIcon
            aria-hidden="true"
            className={cn(
              'size-5 shrink-0 text-muted-foreground transition-transform motion-reduce:transition-none',
              open && 'rotate-90',
            )}
          />
          <span className="min-w-0">
            <span className="block truncate font-medium">{cluster.name}</span>
            <span className="block truncate text-xs text-muted-foreground">
              {cluster.server}
            </span>
          </span>
        </button>
        <Badge variant="muted">{t(`auth.${cluster.auth_mode}`)}</Badge>
        <Badge
          variant={
            checkClass === 'ok' ? 'success' : known ? 'destructive' : 'muted'
          }
        >
          {t(`status.${statusKey}`)}
        </Badge>
        <div className="ml-auto flex items-center gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={test.isPending}
            onClick={() => {
              test.mutate(cluster.name, {
                onError: (e) => {
                  toast.add({ title: e.message, type: 'error' })
                },
              })
            }}
          >
            <PlugZapGlassIcon aria-hidden="true" />
            {test.isPending ? t('actions.testing') : t('actions.test')}
          </Button>
          <RoleGate ability="root">
            <Button
              type="button"
              size="sm"
              variant="outline"
              aria-label={t('actions.disconnectNamed', { name: cluster.name })}
              onClick={() => {
                setConfirming(true)
              }}
            >
              <TrashGlassIcon aria-hidden="true" />
              {t('actions.disconnect')}
            </Button>
          </RoleGate>
        </div>
        {test.data?.cluster_admin ? (
          <p
            role="status"
            className="flex w-full items-center gap-1.5 text-xs text-tone-warning"
          >
            <TriangleAlertGlassIcon aria-hidden="true" className="size-4" />
            {t('adminWarning')}
          </p>
        ) : null}
      </div>
      {open ? <ClusterDetail name={cluster.name} /> : null}
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        tone="destructive"
        title={t('disconnect.title', { name: cluster.name })}
        description={t('disconnect.description')}
        requireTyped={cluster.name}
        confirmLabel={t('disconnect.confirm')}
        pendingLabel={t('disconnect.pending')}
        pending={disconnect.isPending}
        error={disconnect.error?.message ?? null}
        focusCancel
        onConfirm={() => {
          disconnect.mutate(cluster.name, {
            onSuccess: () => {
              setConfirming(false)
              toast.add({ title: t('disconnect.done'), type: 'success' })
            },
          })
        }}
      />
    </li>
  )
}
