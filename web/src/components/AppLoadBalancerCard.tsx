import { useState } from 'react'
import { ArrowsSplitIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { HelpLink } from '@/components/HelpLink'
import {
  useAppLoadBalancer,
  useClearAppLoadBalancer,
  useSetAppLoadBalancer,
} from '../queries/appLoadBalancer'
import {
  configFromForm,
  formFromConfig,
  validateForm,
  type LbFormState,
} from '../lib/loadBalancer'
import { LoadBalancerAlgorithmFields } from './LoadBalancerAlgorithmFields'
import { LoadBalancerExportDialog } from './LoadBalancerExportDialog'
import { LoadBalancerHealthFields } from './LoadBalancerHealthFields'
import { LoadBalancerTrafficFields } from './LoadBalancerTrafficFields'
import { LoadBalancerUpstreamTable } from './LoadBalancerUpstreamTable'

function RemoveDialog({
  appName,
  disabled,
}: {
  appName: string
  disabled: boolean
}) {
  const [open, setOpen] = useState(false)
  const clear = useClearAppLoadBalancer(appName)

  function handleRemove() {
    clear.mutate(undefined, {
      onSuccess: () => {
        setOpen(false)
        toast.add({
          title: 'Load balancer removed.',
          description: `${appName} routes to a single upstream again.`,
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not remove the load balancer.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger
        render={
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={disabled}
          />
        }
      >
        Remove
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Remove load balancer?</DialogTitle>
          <DialogDescription>
            Domains for {appName} go back to a single upstream. Running replicas
            are not touched.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => setOpen(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={clear.isPending}
            onClick={handleRemove}
          >
            Remove
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

interface Props {
  appName: string
  replicas: number
}

export function AppLoadBalancerCard({ appName, replicas }: Props) {
  const query = useAppLoadBalancer(appName)
  const save = useSetAppLoadBalancer(appName)
  const [draft, setDraft] = useState<LbFormState | null>(null)

  if (query.isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Load balancer</CardTitle>
        </CardHeader>
        <CardContent>
          <Skeleton className="h-24 w-full" />
        </CardContent>
      </Card>
    )
  }
  if (query.isError) {
    return (
      <Card>
        <CardContent role="alert" className="pt-6 text-sm text-destructive">
          Could not load the load balancer: {query.error.message}
        </CardContent>
      </Card>
    )
  }

  const configured = query.data?.configured ?? false
  const form = draft ?? formFromConfig(query.data?.config, replicas)
  const errors = validateForm(form)
  const hasErrors = Object.keys(errors).length > 0
  const editing = configured || draft !== null

  function handleSave() {
    if (hasErrors) return
    save.mutate(configFromForm(form), {
      onSuccess: () => {
        setDraft(null)
        toast.add({
          title: 'Load balancer saved.',
          description: 'The ingress reconciler applies it on its next pass.',
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not save the load balancer.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          <ArrowsSplitIcon className="size-4" />
          Load balancer
          <Badge variant={configured ? 'default' : 'muted'}>
            {configured ? 'Enabled' : 'Off'}
          </Badge>
          <HelpLink path="/load-balancing" label="Load balancing guide" />
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        {!editing ? (
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">
              Spread traffic for {appName} across all of its replicas, with
              health checks, retries and graceful cutovers. Without it, domains
              route to a single upstream.
            </p>
            <Button
              type="button"
              size="sm"
              onClick={() => setDraft(formFromConfig(undefined, replicas))}
            >
              Set up load balancer
            </Button>
          </div>
        ) : (
          <>
            {replicas < 2 ? (
              <p className="text-sm text-amber-700 dark:text-amber-300">
                This app runs {replicas} replica. Raise replicas in deploy
                settings to have something to balance.
              </p>
            ) : null}
            <LoadBalancerAlgorithmFields
              form={form}
              errors={errors}
              onChange={(patch) => setDraft({ ...form, ...patch })}
            />
            <LoadBalancerHealthFields
              form={form}
              errors={errors}
              onChange={(patch) => setDraft({ ...form, ...patch })}
            />
            <LoadBalancerTrafficFields
              form={form}
              errors={errors}
              onChange={(patch) => setDraft({ ...form, ...patch })}
            />
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                size="sm"
                disabled={hasErrors || save.isPending || draft === null}
                onClick={handleSave}
              >
                Save
              </Button>
              {draft !== null ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setDraft(null)
                    save.reset()
                  }}
                >
                  Discard changes
                </Button>
              ) : null}
              {configured ? (
                <>
                  <LoadBalancerExportDialog appName={appName} />
                  <RemoveDialog appName={appName} disabled={save.isPending} />
                </>
              ) : null}
            </div>
          </>
        )}
        {configured ? (
          <div className="space-y-2">
            <h3 className="text-sm font-medium">Upstreams</h3>
            <LoadBalancerUpstreamTable appName={appName} />
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
