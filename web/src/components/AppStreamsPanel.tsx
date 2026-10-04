import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  PlugsIcon,
  PlusIcon,
  TrashIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
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
          <PlugsIcon
            className="size-4 text-muted-foreground"
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
            icon={<PlugsIcon className="size-5" />}
            title={t('empty.title')}
            description={t('empty.description')}
          />
        ) : (
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('table.hostPort')}</TableHead>
                  <TableHead>{t('table.containerPort')}</TableHead>
                  <TableHead>{t('table.protocol')}</TableHead>
                  <TableHead>{t('table.created')}</TableHead>
                  <TableHead className="text-right">
                    {t('table.actions')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {streams.map((stream) => (
                  <StreamRow
                    key={stream.id}
                    appName={appName}
                    stream={stream}
                  />
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        <p className="text-xs text-muted-foreground">{t('page.pendingNote')}</p>
      </CardContent>
    </Card>
  )
}

function StreamRow({
  appName,
  stream,
}: {
  appName: string
  stream: AppStream
}) {
  const { t } = useTranslation('streams')
  const deleteStream = useDeleteAppStream(appName)

  return (
    <TableRow>
      <TableCell className="font-mono text-foreground">
        {stream.host_port}
      </TableCell>
      <TableCell className="font-mono text-muted-foreground">
        {stream.container_port}
      </TableCell>
      <TableCell className="text-muted-foreground uppercase">
        {stream.protocol}
      </TableCell>
      <TableCell className="text-muted-foreground">
        {formatDate(stream.created_at)}
      </TableCell>
      <TableCell className="text-right">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={deleteStream.isPending}
          onClick={() => {
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
          }}
        >
          <TrashIcon aria-hidden="true" />
          {deleteStream.isPending ? t('remove.removing') : t('remove.trigger')}
        </Button>
      </TableCell>
    </TableRow>
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
    <form
      onSubmit={handleSubmit}
      className="flex flex-wrap items-end gap-3 rounded-lg border border-border p-3"
    >
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
        <PlusIcon aria-hidden="true" />
        {createStream.isPending ? t('add.submitting') : t('add.submit')}
      </Button>

      {createStream.isError ? (
        <Alert variant="destructive" className="w-full">
          <WarningIcon aria-hidden="true" />
          <AlertDescription>{createStream.error.message}</AlertDescription>
        </Alert>
      ) : null}
    </form>
  )
}
