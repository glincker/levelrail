import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { StatusBadge } from '@/components/ui/status-badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useCloudAccountActions } from '../../hooks/useCloudAccountActions'
import {
  availableActions,
  canTerminate,
  instanceIps,
  instanceTone,
  needsConfirm,
} from '../../lib/cloudInstances'
import {
  useCloudInstance,
  useCloudInstanceAction,
  useCloudInstances,
  useResizeInstance,
  useTerminateInstance,
} from '../../queries/cloudAccounts'
import type {
  CloudInstance,
  CloudInstanceAction,
} from '../../types/cloudAccounts'
import { CloudInstanceDrawer } from './CloudInstanceDrawer'
import { ResizeInstanceDialog } from './ResizeInstanceDialog'
import { TerminateInstanceDialog } from './TerminateInstanceDialog'

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

export function CloudInstancesTab({
  accountId,
  provider,
}: Readonly<{ accountId: string; provider: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const actions = useCloudAccountActions(accountId)
  const list = useCloudInstances(accountId, actions.view)
  const rows = list.data ?? []
  // The capability matrix is per driver, so one row's detail answers for all.
  const probe = useCloudInstance(accountId, rows[0]?.id ?? null)
  const verbs = probe.data?.verbs ?? []
  const power = useCloudInstanceAction(accountId)
  const resize = useResizeInstance(accountId)
  const terminate = useTerminateInstance(accountId)
  const [open, setOpen] = useState<string | null>(null)
  const [stopping, setStopping] = useState<{
    inst: CloudInstance
    action: CloudInstanceAction
  } | null>(null)
  const [resizing, setResizing] = useState<CloudInstance | null>(null)
  const [terminating, setTerminating] = useState<CloudInstance | null>(null)

  if (!actions.view) {
    return (
      <p className="text-sm text-muted-foreground">{t('instances.noAccess')}</p>
    )
  }
  if (list.isPending) {
    return (
      <p role="status" className="text-sm text-muted-foreground">
        {t('section.loading')}
      </p>
    )
  }
  if (list.isError) {
    return (
      <p role="alert" className="text-sm text-destructive">
        {messageOf(list.error)}
      </p>
    )
  }
  if (rows.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('instances.empty')}</p>
    )
  }

  const run = (inst: CloudInstance, action: CloudInstanceAction) => {
    if (needsConfirm(action)) setStopping({ inst, action })
    else power.mutate({ iid: inst.id, action })
  }

  return (
    <div className="space-y-2">
      {power.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(power.error)}
        </p>
      ) : null}
      <Table density="compact">
        <TableHeader>
          <TableRow>
            <TableHead>{t('instances.col.name')}</TableHead>
            <TableHead>{t('instances.col.state')}</TableHead>
            <TableHead>{t('instances.col.size')}</TableHead>
            <TableHead>{t('instances.col.region')}</TableHead>
            <TableHead>{t('instances.col.ip')}</TableHead>
            <TableHead>{t('instances.col.managed')}</TableHead>
            <TableHead className="text-right">
              {t('instances.col.actions')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((inst) => {
            const acts = availableActions(inst, verbs, actions.operate)
            return (
              <TableRow key={inst.id}>
                <TableCell>
                  <button
                    type="button"
                    className="font-medium hover:underline focus-visible:ring-2 focus-visible:ring-ring"
                    onClick={() => {
                      setOpen(inst.id)
                    }}
                  >
                    {inst.name || inst.id}
                  </button>
                </TableCell>
                <TableCell>
                  <StatusBadge
                    tone={instanceTone(inst.state)}
                    label={t(`instances.state.${inst.state}`, {
                      defaultValue: inst.state,
                    })}
                    form="dot"
                  />
                </TableCell>
                <TableCell className="font-mono text-xs">
                  {inst.shape}
                </TableCell>
                <TableCell className="font-mono text-xs">
                  {inst.region}
                </TableCell>
                <TableCell className="font-mono text-xs">
                  {instanceIps(inst)}
                </TableCell>
                <TableCell>
                  <Badge variant={inst.managed ? 'success' : 'muted'}>
                    {inst.managed
                      ? t('instances.managed')
                      : t('instances.unmanaged')}
                  </Badge>
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap justify-end gap-1">
                    {acts.map((a) => (
                      <Button
                        key={a}
                        size="sm"
                        variant="outline"
                        disabled={power.isPending}
                        onClick={() => {
                          run(inst, a)
                        }}
                      >
                        {t(`instances.action.${a}`)}
                      </Button>
                    ))}
                    {actions.build &&
                    inst.managed &&
                    !inst.read_only &&
                    verbs.includes('resize') ? (
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          resize.reset()
                          setResizing(inst)
                        }}
                      >
                        {t('instances.resize.open')}
                      </Button>
                    ) : null}
                    {canTerminate(actions.delete, inst, verbs) ? (
                      <Button
                        size="sm"
                        variant="outline"
                        className="text-destructive"
                        onClick={() => {
                          terminate.reset()
                          setTerminating(inst)
                        }}
                      >
                        {t('instances.terminate.open')}
                      </Button>
                    ) : null}
                  </div>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
      <CloudInstanceDrawer
        accountId={accountId}
        instanceId={open}
        canEdit={actions.operate}
        onClose={() => {
          setOpen(null)
        }}
      />
      <ConfirmDialog
        open={stopping !== null}
        onOpenChange={(o) => {
          if (!o) setStopping(null)
        }}
        title={t('instances.stop.title', {
          name: stopping?.inst.name || stopping?.inst.id || '',
        })}
        description={t('instances.stop.body')}
        confirmLabel={t(`instances.action.${stopping?.action ?? 'stop'}`)}
        pending={power.isPending}
        onConfirm={() => {
          if (!stopping) return
          power.mutate(
            { iid: stopping.inst.id, action: stopping.action },
            {
              onSuccess: () => {
                setStopping(null)
              },
            },
          )
        }}
      />
      <ResizeInstanceDialog
        instance={resizing}
        provider={provider}
        byType={verbs.includes('resize_by_type')}
        pending={resize.isPending}
        error={resize.isError ? messageOf(resize.error) : null}
        onOpenChange={(o) => {
          if (!o) setResizing(null)
        }}
        onSubmit={(spec) => {
          if (!resizing) return
          resize.mutate(
            { iid: resizing.id, spec },
            {
              onSuccess: () => {
                setResizing(null)
              },
            },
          )
        }}
      />
      <TerminateInstanceDialog
        instance={terminating}
        canKeepBootVolume={verbs.includes('keep_boot_volume')}
        pending={terminate.isPending}
        error={terminate.isError ? messageOf(terminate.error) : null}
        onOpenChange={(o) => {
          if (!o) setTerminating(null)
        }}
        onConfirm={(keep) => {
          if (!terminating) return
          terminate.mutate(
            { iid: terminating.id, keepBootVolume: keep },
            {
              onSuccess: () => {
                setTerminating(null)
              },
            },
          )
        }}
      />
    </div>
  )
}
