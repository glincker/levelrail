import { createFileRoute } from '@tanstack/react-router'
import { type FormEvent, useState } from 'react'
import {
  BanGlassIcon,
  CircleCheckGlassIcon,
  PlusGlassIcon,
  ShieldGlassIcon,
  TrashGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
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
import { HostFirewallCard } from '@/components/HostFirewallCard'
import { ExposureCard } from '@/components/exposure/ExposureCard'
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
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { SettingsPageTitle } from '../../components/settings/SettingsPageTitle'

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
  const { t } = useTranslation('settings')
  const { data: rules } = useFirewallRules()
  const { data: doctor } = useSystemDoctor()
  const firewallCheck = doctor.checks.find((c) => c.code === 'firewall')

  return (
    <PageTemplate
      icon={<ShieldGlassIcon size="1.4em" />}
      title={t('firewallPage.title')}
      description={t('firewallPage.description')}
      actions={<AddFirewallRuleDialog />}
    >
      <HostFirewallCard />
      <ExposureCard />

      {firewallCheck ? (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-sm">
              <ShieldGlassIcon
                className="size-5 text-muted-foreground"
                aria-hidden="true"
              />
              {t('firewallRules.liveStatus')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <DoctorCheckRow check={firewallCheck} />
            <p className="mt-4 text-xs text-muted-foreground">
              {t('firewallRules.liveStatusNote')}
            </p>
          </CardContent>
        </Card>
      ) : null}

      {rules.length === 0 ? (
        <EmptyState
          className="gap-2 border-0 bg-transparent py-4"
          icon={<ShieldGlassIcon className="size-6" />}
          title={t('firewallRules.emptyTitle')}
          description={t('firewallRules.emptyBody')}
          action={<AddFirewallRuleDialog />}
        />
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('firewallRules.cols.action')}</TableHead>
                <TableHead>{t('firewallRules.cols.port')}</TableHead>
                <TableHead>{t('firewallRules.cols.protocol')}</TableHead>
                <TableHead>{t('firewallRules.cols.source')}</TableHead>
                <TableHead>{t('firewallRules.cols.label')}</TableHead>
                <TableHead>{t('firewallRules.cols.added')}</TableHead>
                <TableHead className="text-right">
                  {t('firewallRules.cols.actions')}
                </TableHead>
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
                    {rule.source_cidr || t('firewallRules.anywhere')}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {rule.label || t('firewallRules.none')}
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
    </PageTemplate>
  )
}

function ActionBadge({ action }: { action: FirewallRuleAction }) {
  const { t } = useTranslation('settings')
  if (action === 'deny') {
    return (
      <Badge variant="destructive" className="gap-1">
        <BanGlassIcon className="size-4.5" aria-hidden="true" />
        {t('firewallRules.deny')}
      </Badge>
    )
  }
  return (
    <Badge variant="success" className="gap-1">
      <CircleCheckGlassIcon
        className="size-4.5 text-tone-success"
        aria-hidden="true"
      />
      {t('firewallRules.allow')}
    </Badge>
  )
}

function FirewallPending() {
  return (
    <div className="space-y-6">
      <SettingsPageTitle id="firewall" />
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
  const { t } = useTranslation('settings')
  if (action !== 'deny' && sourceCIDR.trim() === '') {
    return null
  }
  return (
    <Alert variant="destructive">
      <TriangleAlertGlassIcon
        className="text-tone-warning"
        size="1.4em"
        aria-hidden="true"
      />
      <AlertTitle>
        {action === 'deny'
          ? t('firewallRules.closeTitle')
          : t('firewallRules.restrictTitle')}
      </AlertTitle>
      <AlertDescription>
        {action === 'deny'
          ? t('firewallRules.closeBody')
          : t('firewallRules.restrictBody')}
      </AlertDescription>
    </Alert>
  )
}

function AddFirewallRuleDialog() {
  const { t } = useTranslation('settings')
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
            title: t('firewallRules.createdToast', { port: portNum }),
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
        <PlusGlassIcon size="1.4em" aria-hidden="true" />
        {t('firewallRules.addRule')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit} className="space-y-3">
          <DialogHeader>
            <DialogTitle>{t('firewallRules.addTitle')}</DialogTitle>
            <DialogDescription>{t('firewallRules.addBody')}</DialogDescription>
          </DialogHeader>

          <Field>
            <FieldLabel htmlFor="fw-port">
              {t('firewallRules.cols.port')}
            </FieldLabel>
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
              <FieldLabel htmlFor="fw-protocol">
                {t('firewallRules.cols.protocol')}
              </FieldLabel>
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
              <FieldLabel htmlFor="fw-action">
                {t('firewallRules.cols.action')}
              </FieldLabel>
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
                  <SelectItem value="allow">
                    {t('firewallRules.allow')}
                  </SelectItem>
                  <SelectItem value="deny">
                    {t('firewallRules.deny')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </div>

          <Field>
            <FieldLabel htmlFor="fw-source-cidr">
              {t('firewallRules.sourceCidr')}
            </FieldLabel>
            <Input
              id="fw-source-cidr"
              value={sourceCIDR}
              onChange={(e) => {
                setSourceCIDR(e.target.value)
              }}
              placeholder={t('firewallRules.sourceCidrPlaceholder')}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="fw-label">
              {t('firewallRules.labelOptional')}
            </FieldLabel>
            <Input
              id="fw-label"
              value={label}
              onChange={(e) => {
                setLabel(e.target.value)
              }}
              placeholder={t('firewallRules.labelPlaceholder')}
            />
          </Field>

          <CloseWarning action={action} sourceCIDR={sourceCIDR} />

          {createRule.isError ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.4em"
                aria-hidden="true"
              />
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
              {t('firewallRules.cancel')}
            </Button>
            <Button
              type="submit"
              variant={action === 'deny' ? 'destructive' : 'default'}
              disabled={createRule.isPending || port.trim() === ''}
            >
              {createRule.isPending
                ? t('firewallRules.adding')
                : action === 'deny'
                  ? t('firewallRules.addDeny')
                  : t('firewallRules.addRule')}
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
  const { t } = useTranslation('settings')
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
        <TrashGlassIcon size="1.4em" aria-hidden="true" />
        {t('firewallRules.remove')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <TriangleAlertGlassIcon
              className="size-5 text-destructive"
              aria-hidden="true"
            />
            {t('firewallRules.removeTitle')}
          </DialogTitle>
          <DialogDescription>
            {rule.action === 'allow'
              ? t('firewallRules.removeAllowBody', {
                  port: rule.port,
                  protocol: rule.protocol,
                })
              : t('firewallRules.removeDenyBody', {
                  port: rule.port,
                  protocol: rule.protocol,
                })}
          </DialogDescription>
        </DialogHeader>
        {deleteRule.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.4em"
              aria-hidden="true"
            />
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
            {t('firewallRules.cancel')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={deleteRule.isPending}
            onClick={() => {
              deleteRule.mutate(rule.id, {
                onSuccess: () => {
                  toast.add({
                    title: t('firewallRules.removedToast'),
                    type: 'success',
                  })
                  setOpen(false)
                },
              })
            }}
          >
            {deleteRule.isPending
              ? t('firewallRules.removing')
              : t('firewallRules.remove')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
