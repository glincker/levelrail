import { createFileRoute } from '@tanstack/react-router'
import { type FormEvent, useState } from 'react'
import {
  CheckCircleIcon,
  PlusIcon,
  ProhibitIcon,
  ShieldIcon,
  TrashIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  firewallRuleListQueryOptions,
  useCreateFirewallRule,
  useDeleteFirewallRule,
  useFirewallRules,
} from '../../queries/firewallRules'
import {
  systemDoctorQueryOptions,
  useSystemDoctor,
} from '../../queries/systemDoctor'
import type {
  FirewallRuleAction,
  FirewallRuleProtocol,
} from '../../types/firewallRule'
import { DoctorCheckRow } from '@/components/DoctorCheckRow'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { PageHeader } from '@/components/shell/PageHeader'
import { TableSkeleton } from '@/components/ui/table-skeleton'

// Settings-level, not tied to one app: a firewall rule is host-wide, the
// same "account/platform scope, not routes/apps/" reasoning
// registry-credentials.tsx's own doc comment gives for its resource.
export const Route = createFileRoute('/settings/firewall')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(firewallRuleListQueryOptions()),
      queryClient.ensureQueryData(systemDoctorQueryOptions()),
    ]),
  component: FirewallPage,
  pendingComponent: FirewallPending,
})

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

function FirewallPage() {
  const { data: rules } = useFirewallRules()
  const { data: doctor } = useSystemDoctor()
  const firewallCheck = doctor.checks.find((c) => c.code === 'firewall')

  return (
    <div className="space-y-6">
      <PageHeader
        title="Firewall"
        description="Declarative host firewall rules, reconciled onto this node's firewall (ufw)."
        actions={<AddFirewallRuleDialog />}
      />

      {firewallCheck ? (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-sm">
              <ShieldIcon
                className="size-4 text-muted-foreground"
                aria-hidden="true"
              />
              Live firewall status
            </CardTitle>
          </CardHeader>
          <CardContent>
            <DoctorCheckRow check={firewallCheck} />
            <p className="mt-1 text-xs text-muted-foreground">
              From this host's own preflight check. See Settings &rarr; System
              status for the full hardening checklist.
            </p>
          </CardContent>
        </Card>
      ) : null}

      {rules.length === 0 ? (
        <EmptyState
          className="py-12"
          icon={<ShieldIcon className="size-5" />}
          title="No firewall rules configured"
          description="Add a rule to allow or deny traffic to a specific port, optionally restricted to a source CIDR."
          action={<AddFirewallRuleDialog />}
        />
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Action</TableHead>
                <TableHead>Port</TableHead>
                <TableHead>Protocol</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Label</TableHead>
                <TableHead>Added</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rules.map((rule) => (
                <TableRow key={rule.id}>
                  <TableCell>
                    <ActionBadge action={rule.action} />
                  </TableCell>
                  <TableCell className="font-mono text-foreground">
                    {rule.port}
                  </TableCell>
                  <TableCell className="text-muted-foreground uppercase">
                    {rule.protocol}
                  </TableCell>
                  <TableCell className="font-mono text-muted-foreground">
                    {rule.source_cidr || 'Anywhere'}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {rule.label || 'None'}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDate(rule.created_at)}
                  </TableCell>
                  <TableCell className="text-right">
                    <DeleteFirewallRuleDialog rule={rule} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}

function ActionBadge({ action }: { action: FirewallRuleAction }) {
  if (action === 'deny') {
    return (
      <Badge variant="destructive" className="gap-1">
        <ProhibitIcon className="size-3" aria-hidden="true" />
        Deny
      </Badge>
    )
  }
  return (
    <Badge variant="success" className="gap-1">
      <CheckCircleIcon className="size-3" aria-hidden="true" />
      Allow
    </Badge>
  )
}

function FirewallPending() {
  return (
    <div className="space-y-6">
      <h1 className="text-lg font-semibold text-foreground">Firewall</h1>
      <TableSkeleton columnCount={7} />
    </div>
  )
}

// A deny rule, or an allow rule scoped to one source CIDR, always risks
// closing a port something still needs: the warning below is shown
// unconditionally while that shape is selected, not only once the
// server has already refused an unsafe one (safetyCheck below), so the
// warning is available before, not just after, a mistake.
function CloseWarning({
  action,
  sourceCIDR,
}: {
  action: FirewallRuleAction
  sourceCIDR: string
}) {
  if (action !== 'deny' && sourceCIDR.trim() === '') {
    return null
  }
  return (
    <Alert variant="destructive">
      <WarningIcon aria-hidden="true" />
      <AlertTitle>
        {action === 'deny'
          ? 'This rule will close a port'
          : 'This rule restricts a port to one source'}
      </AlertTitle>
      <AlertDescription>
        {action === 'deny'
          ? 'Traffic to this port will be dropped. A rule targeting the management API, agent connections, or ingress is refused automatically.'
          : 'Traffic from outside this CIDR will no longer reach this port. A rule targeting the management API, agent connections, or ingress is refused automatically.'}
      </AlertDescription>
    </Alert>
  )
}

function AddFirewallRuleDialog() {
  const [open, setOpen] = useState(false)
  const [port, setPort] = useState('')
  const [protocol, setProtocol] = useState<FirewallRuleProtocol>('tcp')
  const [sourceCIDR, setSourceCIDR] = useState('')
  const [action, setAction] = useState<FirewallRuleAction>('allow')
  const [label, setLabel] = useState('')
  const createRule = useCreateFirewallRule()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setPort('')
      setProtocol('tcp')
      setSourceCIDR('')
      setAction('allow')
      setLabel('')
      createRule.reset()
    }
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const portNum = Number(port)
    createRule.mutate(
      {
        port: portNum,
        protocol,
        source_cidr: sourceCIDR.trim() || undefined,
        action,
        label: label.trim() || undefined,
      },
      {
        onSuccess: () => {
          toast.add({
            title: `Firewall rule for port ${portNum} created.`,
            type: 'success',
          })
          handleOpenChange(false)
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button />}>
        <PlusIcon aria-hidden="true" />
        Add rule
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit} className="space-y-3">
          <DialogHeader>
            <DialogTitle>Add firewall rule</DialogTitle>
            <DialogDescription>
              Allow or deny traffic to a port, optionally restricted to a source
              CIDR. Applied on the next reconcile pass.
            </DialogDescription>
          </DialogHeader>

          <Field>
            <FieldLabel htmlFor="fw-port">Port</FieldLabel>
            <Input
              id="fw-port"
              type="number"
              min={1}
              max={65535}
              value={port}
              onChange={(e) => {
                setPort(e.target.value)
              }}
              placeholder="8443"
            />
          </Field>

          <div className="grid grid-cols-2 gap-3">
            <Field>
              <FieldLabel htmlFor="fw-protocol">Protocol</FieldLabel>
              <Select
                value={protocol}
                onValueChange={(value) => {
                  setProtocol((value as FirewallRuleProtocol) ?? 'tcp')
                }}
              >
                <SelectTrigger id="fw-protocol" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="tcp">TCP</SelectItem>
                  <SelectItem value="udp">UDP</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel htmlFor="fw-action">Action</FieldLabel>
              <Select
                value={action}
                onValueChange={(value) => {
                  setAction((value as FirewallRuleAction) ?? 'allow')
                }}
              >
                <SelectTrigger id="fw-action" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="allow">Allow</SelectItem>
                  <SelectItem value="deny">Deny</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </div>

          <Field>
            <FieldLabel htmlFor="fw-source-cidr">
              Source CIDR (optional)
            </FieldLabel>
            <Input
              id="fw-source-cidr"
              value={sourceCIDR}
              onChange={(e) => {
                setSourceCIDR(e.target.value)
              }}
              placeholder="10.0.0.0/24 (leave blank for any source)"
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="fw-label">Label (optional)</FieldLabel>
            <Input
              id="fw-label"
              value={label}
              onChange={(e) => {
                setLabel(e.target.value)
              }}
              placeholder="what this rule is for"
            />
          </Field>

          <CloseWarning action={action} sourceCIDR={sourceCIDR} />

          {createRule.isError ? (
            <Alert variant="destructive">
              <WarningIcon aria-hidden="true" />
              <AlertDescription>{createRule.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                handleOpenChange(false)
              }}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              variant={action === 'deny' ? 'destructive' : 'default'}
              disabled={createRule.isPending || port.trim() === ''}
            >
              {createRule.isPending
                ? 'Adding...'
                : action === 'deny'
                  ? 'Add deny rule'
                  : 'Add rule'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function DeleteFirewallRuleDialog({
  rule,
}: {
  rule: {
    id: string
    port: number
    protocol: string
    action: FirewallRuleAction
  }
}) {
  const [open, setOpen] = useState(false)
  const deleteRule = useDeleteFirewallRule()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteRule.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <TrashIcon aria-hidden="true" />
        Remove
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <WarningIcon
              className="size-4 text-destructive"
              aria-hidden="true"
            />
            Remove this rule?
          </DialogTitle>
          <DialogDescription>
            {rule.action === 'allow'
              ? `This will stop allowing traffic to port ${rule.port}/${rule.protocol} once the next reconcile pass removes it, unless something else on this host still allows it.`
              : `This will stop denying traffic to port ${rule.port}/${rule.protocol} once the next reconcile pass removes it.`}
          </DialogDescription>
        </DialogHeader>
        {deleteRule.isError ? (
          <Alert variant="destructive">
            <WarningIcon aria-hidden="true" />
            <AlertDescription>{deleteRule.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={deleteRule.isPending}
            onClick={() => {
              deleteRule.mutate(rule.id, {
                onSuccess: () => {
                  toast.add({
                    title: 'Firewall rule removed.',
                    type: 'success',
                  })
                  setOpen(false)
                },
              })
            }}
          >
            {deleteRule.isPending ? 'Removing...' : 'Remove'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
