import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  CircleDashedIcon,
  HourglassIcon,
  PauseCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
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
import { toast } from '@/components/ui/toast'
import {
  usePluginAction,
  useRemovePlugin,
  type Plugin,
} from '../../queries/plugins'

function StateBadge({ plugin }: Readonly<{ plugin: Plugin }>) {
  const { t } = useTranslation('plugins')
  switch (plugin.state) {
    case 'enabled':
      return plugin.running ? (
        <StatusBadge
          tone="success"
          icon={CheckCircleIcon}
          label={t('state.running')}
        />
      ) : (
        <StatusBadge
          tone="warning"
          icon={HourglassIcon}
          label={t('state.stopped')}
        />
      )
    case 'installed':
      return (
        <StatusBadge
          tone="warning"
          icon={CircleDashedIcon}
          label={t('state.installed')}
        />
      )
    case 'removing':
      return (
        <StatusBadge
          tone="danger"
          icon={HourglassIcon}
          label={t('state.removing')}
        />
      )
    default:
      return (
        <StatusBadge
          tone="neutral"
          icon={PauseCircleIcon}
          label={t('state.disabled')}
        />
      )
  }
}

function Abilities({ list }: Readonly<{ list: string[] }>) {
  const { t } = useTranslation('plugins')
  if (list.length === 0) {
    return <span className="text-muted-foreground">{t('none')}</span>
  }
  return (
    <span className="flex flex-wrap gap-1">
      {list.map((a) => (
        <code key={a} className="rounded bg-muted px-1 py-0.5 text-xs">
          {a}
        </code>
      ))}
    </span>
  )
}

export function PluginTable({
  plugins,
  onReview,
}: Readonly<{ plugins: Plugin[]; onReview: (p: Plugin) => void }>) {
  const { t } = useTranslation('plugins')
  const enable = usePluginAction('enable')
  const disable = usePluginAction('disable')
  const remove = useRemovePlugin()
  const [target, setTarget] = useState<Plugin | null>(null)
  const fail = (error: unknown) =>
    toast.add({
      type: 'error',
      title: t('actions.failed'),
      description: error instanceof Error ? error.message : undefined,
    })

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('columns.name')}</TableHead>
            <TableHead>{t('columns.state')}</TableHead>
            <TableHead>{t('columns.requested')}</TableHead>
            <TableHead>{t('columns.approved')}</TableHead>
            <TableHead className="text-right">
              <span className="sr-only">{t('columns.actions')}</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {plugins.map((p) => {
            const approved = p.approved_at !== ''
            return (
              <TableRow key={p.name}>
                <TableCell>
                  <p className="font-medium">
                    {p.name} <span className="font-normal">{p.version}</span>
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t('publisher', { publisher: p.publisher })}
                  </p>
                  {p.last_error ? (
                    <p className="mt-1 text-xs text-destructive">
                      {t('lastError', { message: p.last_error })}
                    </p>
                  ) : null}
                </TableCell>
                <TableCell>
                  <StateBadge plugin={p} />
                </TableCell>
                <TableCell>
                  <Abilities list={p.requested_abilities} />
                </TableCell>
                <TableCell>
                  <Abilities list={p.approved_abilities} />
                </TableCell>
                <TableCell className="space-x-2 text-right whitespace-nowrap">
                  {approved ? null : (
                    <Button type="button" size="sm" onClick={() => onReview(p)}>
                      {t('actions.review')}
                    </Button>
                  )}
                  {approved &&
                  p.state !== 'enabled' &&
                  p.state !== 'removing' ? (
                    <Button
                      type="button"
                      size="sm"
                      disabled={enable.isPending}
                      onClick={() =>
                        enable.mutate(p.name, {
                          onSuccess: () =>
                            toast.add({
                              type: 'success',
                              title: t('actions.enabled', { name: p.name }),
                            }),
                          onError: fail,
                        })
                      }
                    >
                      {t('actions.enable')}
                    </Button>
                  ) : null}
                  {p.state === 'enabled' ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={disable.isPending}
                      onClick={() =>
                        disable.mutate(p.name, {
                          onSuccess: () =>
                            toast.add({
                              type: 'success',
                              title: t('actions.disabled', { name: p.name }),
                            }),
                          onError: fail,
                        })
                      }
                    >
                      {t('actions.disable')}
                    </Button>
                  ) : null}
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => setTarget(p)}
                  >
                    {t('actions.remove')}
                  </Button>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open) setTarget(null)
        }}
        title={t('remove.title', { name: target?.name ?? '' })}
        description={t('remove.description')}
        confirmLabel={t('remove.confirm')}
        tone="destructive"
        pending={remove.isPending}
        error={remove.error?.message ?? null}
        onConfirm={() => {
          if (!target) return
          remove.mutate(target.name, {
            onSuccess: () => {
              toast.add({
                type: 'success',
                title: t('actions.removed', { name: target.name }),
              })
              setTarget(null)
            },
          })
        }}
      />
    </>
  )
}
