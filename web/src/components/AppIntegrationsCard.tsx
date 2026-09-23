import { useState } from 'react'
import { PlugsConnectedIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { AttachIntegrationDialog } from './AttachIntegrationDialog'
import { DisconnectConnectionDialog } from './ConnectionCard'
import {
  useAppIntegrations,
  useDetachAppIntegration,
} from '../queries/appIntegrations'
import type { AppIntegration } from '../types/appIntegrations'

// Third-party tool add-ons (PostHog, Sentry, etc), distinct from the
// bucket/log-drain/database attachment cards this same Integrations tab
// already hosts: those attach an external *resource*, this attaches a
// curated internal/integrations catalog entry as env-var injection only.
export function AppIntegrationsCard({ appName }: { appName: string }) {
  const integrationsQuery = useAppIntegrations(appName)
  const detach = useDetachAppIntegration(appName)
  const [detachTarget, setDetachTarget] = useState<AppIntegration | null>(null)

  const attached = integrationsQuery.data ?? []

  function handleDetach() {
    if (!detachTarget) return
    detach.mutate(detachTarget.id, {
      onSuccess: () => {
        toast.add({
          title: 'Integration detached.',
          description: `${detachTarget.name}'s env vars are removed at ${appName}'s next deploy.`,
          type: 'success',
        })
        setDetachTarget(null)
      },
      onError: (error) => {
        toast.add({
          title: 'Could not detach integration.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <CardTitle>Third-party tools</CardTitle>
        <AttachIntegrationDialog
          appName={appName}
          attachedKeys={attached.map((a) => a.integration_key)}
        />
      </CardHeader>
      <CardContent>
        {integrationsQuery.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : attached.length === 0 ? (
          <EmptyState
            icon={<PlugsConnectedIcon className="size-5" />}
            title="No integrations attached"
            description="Attach PostHog, Sentry, and other curated tools to inject their env vars automatically at deploy time."
          />
        ) : (
          <ul className="space-y-2">
            {attached.map((integration) => (
              <li
                key={integration.id}
                className="flex items-center justify-between gap-4 rounded-lg border border-border bg-card p-2.5"
              >
                <div className="flex items-center gap-2">
                  <PlugsConnectedIcon
                    className="size-4 shrink-0 text-muted-foreground"
                    aria-hidden="true"
                  />
                  <span className="text-sm font-medium text-foreground">
                    {integration.name}
                  </span>
                </div>
                <DisconnectConnectionDialog
                  open={detachTarget?.id === integration.id}
                  onOpenChange={(next) => {
                    setDetachTarget(next ? integration : null)
                  }}
                  title="Detach integration?"
                  description={
                    <>
                      {appName} stops receiving {integration.name}&apos;s env
                      vars at its next container start. Nothing on{' '}
                      {integration.name}&apos;s own side is changed.
                    </>
                  }
                  pending={detach.isPending}
                  actionLabel="Detach"
                  pendingLabel="Detaching..."
                  onConfirm={handleDetach}
                />
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
