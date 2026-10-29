import { type FormEvent, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  PlugGlassIcon,
  PlusGlassIcon,
  TrashGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  useAppStreams,
  useCreateAppStream,
  useDeleteAppStream,
} from '../queries/appStreams'
import type { AppStream } from '../types/appStream'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

// AppStreamsPanel is this first slice's whole UI: a table plus an
// inline add-row, not a dialog (DomainEditor's react-hook-form/zod
// shape is more machinery than one numeric-ports-only form needs).
// Mirrors settings/firewall.tsx's card/table/empty-state layout, scoped
// to one app instead of the whole host.
export function AppStreamsPanel({ appName }: { appName: string }) {
  const { t } = useTranslation('streams')
  const { data: streams } = useAppStreams(appName)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <PlugGlassIcon
            className="size-5 text-muted-foreground"
            aria-hidden="true"
          />
          {t('page.title')}
        </CardTitle>
        <CardDescription>{t('page.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <AddStreamForm appName={appName} />

        {streams.length === 0 ? (
          <EmptyState
            className="py-8"
            icon={<PlugGlassIcon className="size-6" />}
            title={t('empty.title')}
            description={t('empty.description')}
          />
        ) : (
          <StreamsTable appName={appName} streams={streams} />
        )}

        <p className="text-xs text-muted-foreground">{t('page.pendingNote')}</p>
      </CardContent>
    </Card>
  )
}

function StreamsTable({
  appName,
  streams,
}: {
  appName: string
  streams: readonly AppStream[]
}) {
  const { t } = useTranslation('streams')
  const deleteStream = useDeleteAppStream(appName)
  const columns = useMemo<ResponsiveColumn<AppStream>[]>(
    () => [
      {
        id: 'hostPort',
        primary: true,
        header: t('table.hostPort'),
        cell: (s) => <span className="font-mono">{s.host_port}</span>,
      },
      {
        id: 'containerPort',
        header: t('table.containerPort'),
        cell: (s) => (
          <span className="font-mono text-muted-foreground">
            {s.container_port}
          </span>
        ),
      },
      {
        id: 'protocol',
        header: t('table.protocol'),
        priority: 2,
        cell: (s) => (
          <span className="text-muted-foreground uppercase">{s.protocol}</span>
        ),
      },
      {
        id: 'created',
        header: t('table.created'),
        priority: 3,
        cell: (s) => (
          <span className="text-muted-foreground">
            {formatDate(s.created_at)}
          </span>
        ),
      },
    ],
    [t],
  )

  function remove(stream: AppStream) {
    deleteStream.mutate(stream.id, {
      onSuccess: () => {
        toast.add({ title: t('toast.removed'), type: 'success' })
      },
      onError: (error) => {
        toast.add({
          title: t('toast.errorTitle'),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <ResponsiveTable
      rows={streams}
      columns={columns}
      getRowId={(s) => s.id}
      ariaLabel={t('tableLabel')}
      renderActions={(stream) => (
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={deleteStream.isPending}
          onClick={() => {
            remove(stream)
          }}
        >
          <TrashGlassIcon size="1.4em" aria-hidden="true" />
          {deleteStream.isPending ? t('remove.removing') : t('remove.trigger')}
        </Button>
      )}
    />
  )
}

function AddStreamForm({ appName }: { appName: string }) {
  const { t } = useTranslation('streams')
  const [hostPort, setHostPort] = useState('')
  const [containerPort, setContainerPort] = useState('')
  const createStream = useCreateAppStream(appName)

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const hostPortNum = Number(hostPort)
    const containerPortNum = Number(containerPort)
    createStream.mutate(
      {
        host_port: hostPortNum,
        container_port: containerPortNum,
        protocol: 'tcp',
      },
      {
        onSuccess: () => {
          toast.add({
            title: t('toast.created', { hostPort: hostPortNum }),
            type: 'success',
          })
          setHostPort('')
          setContainerPort('')
        },
        onError: (error) => {
          toast.add({
            title: t('toast.errorTitle'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-wrap items-end gap-3">
      <Field className="w-32">
        <FieldLabel htmlFor="stream-host-port">
          {t('add.hostPortLabel')}
        </FieldLabel>
        <Input
          id="stream-host-port"
          type="number"
          min={1}
          max={65535}
          value={hostPort}
          onChange={(e) => {
            setHostPort(e.target.value)
          }}
          placeholder={t('add.hostPortPlaceholder')}
        />
      </Field>
      <Field className="w-36">
        <FieldLabel htmlFor="stream-container-port">
          {t('add.containerPortLabel')}
        </FieldLabel>
        <Input
          id="stream-container-port"
          type="number"
          min={1}
          max={65535}
          value={containerPort}
          onChange={(e) => {
            setContainerPort(e.target.value)
          }}
          placeholder={t('add.containerPortPlaceholder')}
        />
      </Field>
      <Field className="w-24">
        <FieldLabel htmlFor="stream-protocol">
          {t('add.protocolLabel')}
        </FieldLabel>
        <Input id="stream-protocol" value="TCP" disabled readOnly />
      </Field>
      <Button
        type="submit"
        disabled={
          createStream.isPending ||
          hostPort.trim() === '' ||
          containerPort.trim() === ''
        }
      >
        <PlusGlassIcon size="1.4em" aria-hidden="true" />
        {createStream.isPending ? t('add.submitting') : t('add.submit')}
      </Button>

      {createStream.isError ? (
        <Alert variant="destructive" className="w-full">
          <TriangleAlertGlassIcon
            className="text-tone-warning"
            size="1.7em"
            aria-hidden="true"
          />
          <AlertDescription>{createStream.error.message}</AlertDescription>
        </Alert>
      ) : null}
    </form>
  )
}
