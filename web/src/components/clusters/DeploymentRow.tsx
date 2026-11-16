import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ScrollTextGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { RoleGate } from '@/components/ui/RoleGate'
import { toast } from '@/components/ui/toast'
import { Badge } from '@/components/ui/badge'
import {
  useDeploymentStatus,
  useRemoveDeployment,
} from '../../queries/clusters'
import { DeploymentLogsSheet } from './DeploymentLogsSheet'
import type { K8sDeployment, K8sPod } from '../../types/clusters'

// The most useful failing pod wins: one with a reason, such as CrashLoopBackOff.
function crashPod(pods: readonly K8sPod[]): K8sPod | undefined {
  return pods.find((p) => p.reason) ?? undefined
}

export function DeploymentRow({
  cluster,
  deployment,
}: Readonly<{ cluster: string; deployment: K8sDeployment }>) {
  const { t } = useTranslation('clusters')
  const [logsOpen, setLogsOpen] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const remove = useRemoveDeployment(cluster)
  const status = useDeploymentStatus(cluster, deployment.app)
  const s = status.data
  const bad = s && !s.ready ? crashPod(s.pods) : undefined

  return (
    <li className="flex flex-col gap-1 px-3 py-2 text-sm">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="min-w-0 flex-1 basis-40 truncate font-medium">
          {deployment.app}
        </span>
        {status.isPending ? (
          <Badge variant="muted">{t('deployment.checking')}</Badge>
        ) : status.isError || !s ? (
          <Badge variant="muted">{t('deployment.stale')}</Badge>
        ) : (
          <Badge
            variant={s.ready ? 'success' : bad ? 'destructive' : 'warning'}
          >
            {t('deployment.ready', {
              ready: s.ready_replicas,
              total: s.replicas,
            })}
          </Badge>
        )}
        {bad ? <Badge variant="destructive">{bad.reason}</Badge> : null}
        <span className="min-w-0 truncate text-xs text-muted-foreground">
          {deployment.namespace} · {deployment.image}
        </span>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="ml-auto"
          aria-label={t('deployment.logsNamed', { name: deployment.app })}
          onClick={() => {
            setLogsOpen(true)
          }}
        >
          <ScrollTextGlassIcon aria-hidden="true" />
          {t('deployment.logs')}
        </Button>
        <RoleGate ability="root">
          <Button
            type="button"
            size="sm"
            variant="outline"
            aria-label={t('deployment.removeNamed', { name: deployment.app })}
            onClick={() => {
              setConfirming(true)
            }}
          >
            <TrashGlassIcon aria-hidden="true" />
            {t('deployment.remove')}
          </Button>
        </RoleGate>
        {s?.ingress_address ? (
          <span className="text-xs text-muted-foreground tabular-nums">
            {s.ingress_address}
          </span>
        ) : null}
      </div>
      {bad?.message ? (
        <p className="text-xs break-words text-muted-foreground">
          {bad.message}
        </p>
      ) : null}
      {s && !s.ready && !bad && s.reason ? (
        <p className="text-xs text-muted-foreground">{s.reason}</p>
      ) : null}
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        tone="destructive"
        title={t('remove.title', { name: deployment.app })}
        description={t('remove.description')}
        requireTyped={deployment.app}
        confirmLabel={t('remove.confirm')}
        pendingLabel={t('remove.pending')}
        pending={remove.isPending}
        error={remove.error?.message ?? null}
        focusCancel
        onConfirm={() => {
          remove.mutate(deployment.app, {
            onSuccess: () => {
              setConfirming(false)
              toast.add({
                title: t('remove.done', { name: deployment.app }),
                type: 'success',
              })
            },
          })
        }}
      />
      <DeploymentLogsSheet
        cluster={cluster}
        app={deployment.app}
        pods={s?.pods ?? []}
        open={logsOpen}
        onOpenChange={setLogsOpen}
      />
    </li>
  )
}
