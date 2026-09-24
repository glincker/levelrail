import { createFileRoute, Link } from '@tanstack/react-router'
import {
  CheckCircleIcon,
  WarningCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import { useAttentionItems } from '../queries/attention'
import { useRestartApp } from '../queries/apps'
import { useTriggerDeploy } from '../queries/deploys'
import type { AttentionItem } from '../lib/attention'

export const Route = createFileRoute('/status')({
  component: StatusPage,
})

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
  const { target } = item

  if (target.kind === 'app') {
    return (
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          variant="outline"
          disabled={restart.isPending}
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
          render={<Link to="/apps/$name/logs" params={{ name: target.name }} />}
        >
          View logs
        </Button>
        <Button
          size="sm"
          variant="outline"
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
        render={<Link to="/nodes/$id" params={{ id: target.id }} />}
      >
        Open node
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

function StatusPage() {
  const { items, isLoading } = useAttentionItems()

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-semibold">Status</h1>
        <p className="text-sm text-muted-foreground">
          Everything that needs attention right now. Refreshes every 30 seconds.
        </p>
      </div>

      {isLoading ? (
        <div className="h-24 animate-pulse rounded-lg bg-muted" />
      ) : items.length === 0 ? (
        <EmptyState
          icon={<CheckCircleIcon className="size-5" />}
          title="All systems healthy"
          description="No failing apps or deploys, offline nodes, low disk, or expiring certificates."
        />
      ) : (
        <ul className="space-y-2">
          {items.map((item) => {
            const Icon =
              item.severity === 'critical' ? WarningCircleIcon : WarningIcon
            return (
              <li
                key={item.id}
                className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border bg-card p-3"
              >
                <div className="flex min-w-0 items-start gap-2.5">
                  <Icon
                    className={`mt-0.5 size-4 shrink-0 ${item.severity === 'critical' ? 'text-destructive' : 'text-amber-600 dark:text-amber-400'}`}
                    aria-hidden="true"
                  />
                  <div className="min-w-0">
                    <p className="flex items-center gap-2 text-sm font-medium">
                      {item.title}
                      <Badge
                        variant={
                          item.severity === 'critical' ? 'destructive' : 'muted'
                        }
                      >
                        {item.severity}
                      </Badge>
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {item.detail}
                    </p>
                  </div>
                </div>
                <ItemActions item={item} />
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
