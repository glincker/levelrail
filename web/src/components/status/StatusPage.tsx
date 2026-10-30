import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ActivityGlassIcon,
  CircleAlertGlassIcon,
  InfoGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { PageBanner } from '@/components/PageBanner'
import { LazyStatusDiagram } from '@/components/banners/LazyPass3Diagrams'
import { toast } from '@/components/ui/toast'
import { useAttentionItems } from '@/queries/attention'
import { useRestartApp } from '@/queries/apps'
import { useTriggerDeploy } from '@/queries/deploys'
import type { AttentionItem } from '@/lib/attention'
import { ResolvedLoginControls } from '@/components/attention/ResolvedLoginControls'
import { CopyFailureDetail } from '@/components/attention/CopyFailureDetail'
import { CopyItemControls } from '@/components/attention/CopyItemControls'
import { useSnoozed } from '@/components/shell/snoozeStore'
import { SilenceMenu } from '@/components/attention/SilenceMenu'
import { IncidentsSection } from '@/components/incidents/IncidentsSection'
import { MonitorsSection } from '@/components/monitors/MonitorsSection'
import { SilencedSection } from '@/components/attention/SilencedSection'

function FailedDeployActions({
  app,
  image,
  lastGoodImage,
}: {
  app: string
  image: string
  lastGoodImage?: string
}) {
  const deploy = useTriggerDeploy(app)
  const run = (target: string, label: string) => {
    deploy.mutate(
      { image: target },
      {
        onSuccess: () => {
          toast.add({ title: `${label} ${app}`, type: 'success' })
        },
        onError: (err) => {
          toast.add({ title: err.message, type: 'error' })
        },
      },
    )
  }
  return (
    <div className="flex flex-wrap gap-2">
      <Button
        size="sm"
        variant="outline"
        disabled={deploy.isPending}
        onClick={() => run(image, 'Redeploying')}
      >
        Redeploy
      </Button>
      {lastGoodImage ? (
        <Button
          size="sm"
          variant="outline"
          disabled={deploy.isPending}
          onClick={() => run(lastGoodImage, 'Rolling back')}
        >
          Rollback
        </Button>
      ) : null}
      <Button
        size="sm"
        variant="outline"
        render={<Link to="/apps/$name/deploys" params={{ name: app }} />}
      >
        Deploys
      </Button>
    </div>
  )
}

function ItemActions({ item }: { item: AttentionItem }) {
  const restart = useRestartApp()
  const { t } = useTranslation('attention')
  const { target } = item

  if (target.kind === 'app') {
    return (
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          variant="outline"
          disabled={restart.isPending}
          aria-label={`Restart ${target.name}`}
          onClick={() => {
            restart.mutate(target.name, {
              onSuccess: () => {
                toast.add({
                  title: `Restarting ${target.name}`,
                  type: 'success',
                })
              },
              onError: (err) => {
                toast.add({ title: err.message, type: 'error' })
              },
            })
          }}
        >
          {restart.isPending ? 'Restarting...' : 'Restart'}
        </Button>
        <Button
          size="sm"
          variant="outline"
          aria-label={`View logs for ${target.name}`}
          render={<Link to="/apps/$name/logs" params={{ name: target.name }} />}
        >
          View logs
        </Button>
        <Button
          size="sm"
          variant="outline"
          aria-label={`Deploys for ${target.name}`}
          render={
            <Link to="/apps/$name/deploys" params={{ name: target.name }} />
          }
        >
          Deploys
        </Button>
      </div>
    )
  }
  if (target.kind === 'deploy') {
    return (
      <FailedDeployActions
        app={target.app}
        image={target.image}
        lastGoodImage={target.lastGoodImage}
      />
    )
  }
  if (target.kind === 'node') {
    return (
      <Button
        size="sm"
        variant="outline"
        aria-label={`Open node ${target.id}`}
        render={<Link to="/nodes/$id" params={{ id: target.id }} />}
      >
        Open node
      </Button>
    )
  }
  if (item.device?.dismissKey && item.device.auditPath) {
    return (
      <ResolvedLoginControls
        dismissKey={item.device.dismissKey}
        auditPath={item.device.auditPath}
      />
    )
  }
  if (target.kind === 'path' && item.copy) {
    return (
      <CopyItemControls
        itemId={item.id}
        dismissKey={item.copy.dismissKey}
        to={target.to}
      />
    )
  }
  if (target.kind === 'path' && item.id.startsWith('node_reenroll:')) {
    return (
      <Button size="sm" variant="outline" render={<Link to={target.to} />}>
        {t('items.reenrollNode')}
      </Button>
    )
  }
  if (target.kind === 'path') {
    return (
      <Button size="sm" variant="outline" render={<Link to={target.to} />}>
        {t('items.openItem')}
      </Button>
    )
  }
  if (target.kind === 'route') {
    return (
      <Button size="sm" variant="outline" render={<Link to={target.to} />}>
        {item.id.startsWith('device:')
          ? t('items.openCliAccess')
          : t('items.openApprovals')}
      </Button>
    )
  }
  if (target.kind === 'domain') {
    return (
      <Button size="sm" variant="outline" render={<Link to="/domains" />}>
        Open domains
      </Button>
    )
  }
  return (
    <Button
      size="sm"
      variant="outline"
      render={<Link to="/settings/system-status" />}
    >
      Open system status
    </Button>
  )
}

function severityIcon(severity: AttentionItem['severity']) {
  if (severity === 'critical') return CircleAlertGlassIcon
  return severity === 'info' ? InfoGlassIcon : TriangleAlertGlassIcon
}

export function StatusPage() {
  const { items: all, isLoading } = useAttentionItems()
  const { t } = useTranslation('attention')
  const { isSnoozed, silences, now } = useSnoozed()
  const items = all.filter((i) => !isSnoozed(i.id))

  const healthy = !isLoading && items.length === 0
  const size = healthy ? 'hero' : 'compact'

  return (
    <PageTemplate
      title={t('statusPage.title')}
      banner={
        <PageBanner
          icon={<ActivityGlassIcon size="1.4em" />}
          titleKey="attention:statusPage.title"
          descriptionKey={
            healthy
              ? 'attention:statusPage.healthyDescription'
              : 'attention:statusPage.description'
          }
          illustration={<LazyStatusDiagram size={size} />}
          size={size}
        />
      }
    >
      {isLoading ? (
        <div
          role="status"
          aria-label={t('statusPage.loading', {
            defaultValue: 'Loading status',
          })}
          className="h-24 animate-pulse rounded-lg bg-muted motion-reduce:animate-none"
        />
      ) : items.length === 0 ? null : (
        <ul className="space-y-2">
          {items.map((item) => {
            const Icon = severityIcon(item.severity)
            return (
              <li
                key={item.id}
                className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border bg-card p-3"
              >
                <div className="flex min-w-0 flex-1 items-start gap-2.5">
                  <Icon
                    className={`size-6 shrink-0 ${item.severity === 'critical' ? 'text-destructive' : item.severity === 'info' ? 'text-muted-foreground' : 'text-amber-600 dark:text-amber-400'}`}
                    aria-hidden="true"
                  />
                  <div className="min-w-0 flex-1">
                    <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm font-medium">
                      <span className="min-w-0 break-words">{item.title}</span>
                      <Badge
                        variant={
                          item.severity === 'critical' ? 'destructive' : 'muted'
                        }
                      >
                        {item.device
                          ? t(`deviceLogin.state.${item.device.state}`)
                          : item.severity}
                      </Badge>
                    </p>
                    <p className="text-xs break-words text-muted-foreground">
                      {item.detail}
                      {item.device?.requesterIp
                        ? ` (${item.device.requesterIp})`
                        : ''}
                    </p>
                    {item.copy ? (
                      <CopyFailureDetail
                        hint={item.copy.hint}
                        technical={item.copy.technical}
                      />
                    ) : item.action ? (
                      <p className="text-xs break-words text-muted-foreground">
                        {item.action}
                      </p>
                    ) : null}
                  </div>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <ItemActions item={item} />
                  {item.severity === 'info' ? null : (
                    <SilenceMenu
                      issueKey={item.id}
                      severity={item.severity}
                      title={item.title}
                    />
                  )}
                </div>
              </li>
            )
          })}
        </ul>
      )}
      <MonitorsSection />
      <IncidentsSection />
      <SilencedSection
        items={all}
        silences={silences}
        now={now}
        isSnoozed={isSnoozed}
      />
    </PageTemplate>
  )
}
