import { useState } from 'react'
import {
  CheckCircleIcon,
  LockIcon,
  LockOpenIcon,
  PlusIcon,
  QuestionIcon,
  WarningCircleIcon,
  WarningIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { HelpLink } from '@/components/HelpLink'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Field, FieldError, FieldGroup } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import type { VariantProps } from 'class-variance-authority'
import type { ConditionStatus } from '../types/deploy'
import {
  useAppEgressPolicy,
  useClearAppEgressPolicy,
  useSetAppEgressPolicy,
} from '../queries/appEgress'
import { useDeployStatus } from '../queries/deploys'

type RuleRow = { host: string; port: string }

function toRuleRows(
  allow: { host: string; port: number }[] | undefined,
): RuleRow[] {
  if (!allow || allow.length === 0) return [{ host: '', port: '' }]
  return allow.map((a) => ({ host: a.host, port: String(a.port) }))
}

// Translated status for EgressPolicyReady's own reason strings
// (internal/reconcile/application/egress.go): what an operator should
// take away, not the raw machine reason.
const STATUS_BADGE_VARIANT: Record<
  ConditionStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  True: 'success',
  False: 'destructive',
  Unknown: 'muted',
}

const STATUS_ICON = {
  True: CheckCircleIcon,
  False: WarningCircleIcon,
  Unknown: QuestionIcon,
}

function liveStatusLabel(reason: string): string {
  switch (reason) {
    case 'PoliciesApplied':
      return 'Enforced'
    case 'PolicyApplyFailed':
      return 'Not enforced'
    case 'NoRunningTargets':
      return 'Nothing running to protect yet'
    case 'NotConfigured':
    default:
      return 'Not configured'
  }
}

// AppEgressPolicyCard is the outbound half of the Network page
// (network.tsx), rendered below AppNetworkPanel's own inbound
// traffic-path card: GET/PUT/DELETE /api/v1/apps/{name}/egress-policy
// (internal/api/apps_egress.go), an opt-in allowlist that restricts this
// app's container to only reach the declared host:port pairs. A separate
// card, not a section folded into AppNetworkPanel, since that panel's own
// framing ("how traffic reaches this app") is inbound-only and egress is
// a genuinely different mechanism (a sidecar container, not Caddy).
//
// Unconfigured (no policy saved) means unrestricted egress, unchanged
// from today; this card never implies otherwise. Live enforcement status
// (EgressPolicyReady) is read from the same conditions query the parent
// layout route already primes (routes/apps/$name.tsx's loader), so
// rendering this needs no extra request.
export function AppEgressPolicyCard({ appName }: { appName: string }) {
  const policyQuery = useAppEgressPolicy(appName)
  const { data: conditions } = useDeployStatus(appName)
  const setPolicy = useSetAppEgressPolicy(appName)
  const clearPolicy = useClearAppEgressPolicy(appName)
  const [editing, setEditing] = useState(false)
  const [rows, setRows] = useState<RuleRow[]>([{ host: '', port: '' }])
  const [confirmClearOpen, setConfirmClearOpen] = useState(false)

  if (policyQuery.isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <LockOpenIcon className="size-4" />
            Outbound network
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Skeleton className="h-16 w-full" />
        </CardContent>
      </Card>
    )
  }

  const policy = policyQuery.data
  const configured = policy?.mode === 'allowlist'
  const egressCondition = conditions.find((c) => c.Type === 'EgressPolicyReady')
  const pending = setPolicy.isPending || clearPolicy.isPending

  function startEditing() {
    setRows(toRuleRows(policy?.allow))
    setEditing(true)
  }

  function cancelEditing() {
    setEditing(false)
    setPolicy.reset()
  }

  function updateRow(index: number, patch: Partial<RuleRow>) {
    setRows((prev) =>
      prev.map((r, i) => (i === index ? { ...r, ...patch } : r)),
    )
  }

  function removeRow(index: number) {
    setRows((prev) => prev.filter((_, i) => i !== index))
  }

  function addRow() {
    setRows((prev) => [...prev, { host: '', port: '' }])
  }

  function rowError(row: RuleRow): string | undefined {
    if (row.host.trim().length === 0) return 'Host is required'
    const port = Number.parseInt(row.port, 10)
    if (!Number.isFinite(port) || port < 1 || port > 65535) {
      return 'Port must be between 1 and 65535'
    }
    return undefined
  }

  const rowErrors = rows.map(rowError)
  const hasErrors = rowErrors.some((e) => e !== undefined) || rows.length === 0

  function handleSave() {
    if (hasErrors) return
    const allow = rows.map((r) => ({
      host: r.host.trim(),
      port: Number.parseInt(r.port, 10),
    }))
    setPolicy.mutate(
      { mode: 'allowlist', allow },
      {
        onSuccess: () => {
          setEditing(false)
          toast.add({
            title: 'Egress allowlist saved.',
            description:
              'The reconciler will attach an egress sidecar to enforce it shortly.',
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({
            title: 'Could not save egress allowlist.',
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  function handleClear() {
    clearPolicy.mutate(undefined, {
      onSuccess: () => {
        setConfirmClearOpen(false)
        setEditing(false)
        toast.add({
          title: 'Egress restriction removed.',
          description: `${appName} now has unrestricted outbound network access.`,
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not clear egress policy.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {configured ? (
            <LockIcon className="size-4" />
          ) : (
            <LockOpenIcon className="size-4" />
          )}
          Outbound network
          <HelpLink
            path="/deploying-apps#outbound-network-egress-allowlist"
            label="Egress allowlist guide"
          />
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <Badge variant={configured ? 'default' : 'muted'}>
              {configured ? 'Allowlist mode' : 'Unrestricted egress'}
            </Badge>
            {configured && egressCondition ? (
              <StatusBadge
                status={egressCondition.Status}
                reason={egressCondition.Reason}
              />
            ) : null}
          </div>
          {!editing ? (
            <div className="flex items-center gap-2">
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={startEditing}
              >
                {configured ? 'Edit rules' : 'Restrict outbound traffic'}
              </Button>
              {configured ? (
                <Dialog
                  open={confirmClearOpen}
                  onOpenChange={setConfirmClearOpen}
                >
                  <DialogTrigger
                    render={
                      <Button type="button" size="sm" variant="outline" />
                    }
                  >
                    Clear
                  </DialogTrigger>
                  <DialogContent className="sm:max-w-sm">
                    <DialogHeader>
                      <DialogTitle className="flex items-center gap-1.5 text-destructive">
                        <WarningIcon className="size-4" aria-hidden="true" />
                        Remove the egress allowlist?
                      </DialogTitle>
                      <DialogDescription>
                        {appName} will go back to unrestricted outbound network
                        access, reaching any host and port, the same as before
                        this policy was configured. This changes real network
                        behavior for the running container, not just a saved
                        setting.
                      </DialogDescription>
                    </DialogHeader>
                    <DialogFooter>
                      <Button
                        type="button"
                        variant="outline"
                        onClick={() => setConfirmClearOpen(false)}
                      >
                        Cancel
                      </Button>
                      <Button
                        type="button"
                        variant="destructive"
                        disabled={clearPolicy.isPending}
                        onClick={handleClear}
                      >
                        {clearPolicy.isPending
                          ? 'Clearing...'
                          : 'Clear allowlist'}
                      </Button>
                    </DialogFooter>
                  </DialogContent>
                </Dialog>
              ) : null}
            </div>
          ) : null}
        </div>

        {!editing && configured && policy?.allow && policy.allow.length > 0 ? (
          <ul className="flex flex-wrap gap-2">
            {policy.allow.map((rule) => (
              <li key={`${rule.host}:${rule.port}`}>
                <Badge variant="outline" className="font-mono">
                  {rule.host}:{rule.port}
                </Badge>
              </li>
            ))}
          </ul>
        ) : null}

        {!editing && configured && egressCondition?.Message ? (
          <p className="text-sm text-muted-foreground">
            {egressCondition.Message}
          </p>
        ) : null}

        {editing ? (
          <div className="space-y-3">
            <FieldGroup className="gap-2">
              {rows.map((row, index) => (
                <Field key={index} orientation="responsive">
                  <div className="flex-1">
                    <Input
                      value={row.host}
                      onChange={(e) =>
                        updateRow(index, { host: e.target.value })
                      }
                      className="font-mono"
                      placeholder="api.example.com"
                      aria-label="Allowed host"
                    />
                  </div>
                  <div className="flex flex-1 items-start gap-2">
                    <Input
                      value={row.port}
                      onChange={(e) =>
                        updateRow(index, { port: e.target.value })
                      }
                      type="number"
                      min={1}
                      max={65535}
                      className="w-28 font-mono"
                      placeholder="443"
                      aria-label="Allowed port"
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => removeRow(index)}
                      disabled={rows.length === 1}
                    >
                      <XIcon />
                      <span className="sr-only">Remove rule</span>
                    </Button>
                  </div>
                  <FieldError
                    errors={
                      rowErrors[index]
                        ? [{ message: rowErrors[index] }]
                        : undefined
                    }
                  />
                </Field>
              ))}
            </FieldGroup>
            <Button type="button" variant="outline" size="sm" onClick={addRow}>
              <PlusIcon />
              Add rule
            </Button>
            <p className="text-sm text-muted-foreground">
              DNS lookups and loopback traffic always stay open, regardless of
              this list. After a deploy or restart, there is a brief window
              before the egress sidecar finishes attaching where outbound
              traffic is temporarily unrestricted.
            </p>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                size="sm"
                disabled={pending || hasErrors}
                onClick={handleSave}
              >
                {setPolicy.isPending ? 'Saving...' : 'Save allowlist'}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={pending}
                onClick={cancelEditing}
              >
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            {configured
              ? 'DNS lookups and loopback traffic always stay open regardless of this list.'
              : 'This app can reach any host and port. Restrict it to an explicit allowlist to limit what it can reach outbound.'}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

function StatusBadge({
  status,
  reason,
}: {
  status: ConditionStatus
  reason: string
}) {
  const Icon = STATUS_ICON[status]
  return (
    <Badge variant={STATUS_BADGE_VARIANT[status]} className="rounded-full">
      <Icon className="size-3" />
      {liveStatusLabel(reason)}
    </Badge>
  )
}
