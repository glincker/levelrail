import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { useDeploymentStatus } from '../../queries/clusters'
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
    </li>
  )
}
