import { useCallback, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  QrCodeGlassIcon,
  SmartphoneGlassIcon,
} from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/toast'
import { StepUpDialog } from '../../components/env/StepUpDialog'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import { requireExperimental } from '../../lib/experimental'
import {
  createMobilePairingCode,
  MOBILE_PAIRING_STEP_UP_APP,
  mobileDevicesQueryOptions,
  useMobileDevices,
  useRevokeMobileDevice,
  type MobileDevice,
  type MobilePairingCode,
} from '../../queries/mobileDevices'

export const Route = createFileRoute('/settings/mobile-devices')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'mobile-api', location.href),
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(mobileDevicesQueryOptions()),
  component: MobileDevicesPage,
  pendingComponent: () => <TableSkeleton columnCount={6} rowCount={3} />,
})

function CopyRow({ value, label }: { value: string; label: string }) {
  const { t } = useTranslation('settings')
  const [copied, setCopied] = useState(false)
  return (
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="flex items-center gap-2 rounded-lg bg-muted/50 p-2">
        <code className="min-w-0 flex-1 text-xs break-all">{value}</code>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            void navigator.clipboard.writeText(value).then(() => {
              setCopied(true)
              setTimeout(() => setCopied(false), 2000)
            })
          }}
        >
          {copied ? (
            <CheckGlassIcon size="1.2em" aria-hidden="true" />
          ) : (
            <CopyGlassIcon size="1.2em" aria-hidden="true" />
          )}
          {copied ? t('mobileDevices.copied') : t('mobileDevices.copy')}
        </Button>
      </div>
    </div>
  )
}

function formatTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : ''
}

function MobileDevicesPage() {
  const { t } = useTranslation('settings')
  const { data: devices = [], isLoading } = useMobileDevices()
  const revoke = useRevokeMobileDevice()
  const [pairing, setPairing] = useState<MobilePairingCode | null>(null)
  const [target, setTarget] = useState<MobileDevice | null>(null)
  const onError = useCallback(
    (error: unknown) => {
      toast.add({
        type: 'error',
        title: t('mobileDevices.pairFailed'),
        description: error instanceof Error ? error.message : undefined,
      })
    },
    [t],
  )
  const gate = useStepUpGate(onError)

  const startPairing = () => {
    void gate.guard(
      { action: 'manage', app: MOBILE_PAIRING_STEP_UP_APP },
      async () => {
        setPairing(await createMobilePairingCode())
      },
    )
  }

  return (
    <PageTemplate
      icon={<SmartphoneGlassIcon size="1.2em" />}
      title={t('mobileDevices.title')}
      description={t('mobileDevices.description')}
      actions={
        <Button type="button" onClick={startPairing}>
          <QrCodeGlassIcon size="1.2em" aria-hidden="true" />
          {t('mobileDevices.pair')}
        </Button>
      }
    >
      {pairing ? (
        <Card>
          <CardHeader>
            <CardTitle>{t('mobileDevices.pairingTitle')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">
              {t('mobileDevices.pairingHint', {
                time: formatTime(pairing.expires_at),
              })}
            </p>
            <CopyRow
              value={pairing.code}
              label={t('mobileDevices.codeLabel')}
            />
            <CopyRow
              value={pairing.pair_url}
              label={t('mobileDevices.linkLabel')}
            />
            <CopyRow
              value={pairing.server_url}
              label={t('mobileDevices.serverLabel')}
            />
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setPairing(null)}
            >
              {t('mobileDevices.done')}
            </Button>
          </CardContent>
        </Card>
      ) : null}
      {isLoading ? (
        <TableSkeleton columnCount={6} rowCount={3} />
      ) : devices.length === 0 ? (
        <EmptyState
          className="gap-2 border-0 bg-transparent py-4"
          icon={<SmartphoneGlassIcon className="size-5.5" />}
          title={t('mobileDevices.emptyTitle')}
          description={t('mobileDevices.emptyDescription')}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('mobileDevices.colName')}</TableHead>
              <TableHead>{t('mobileDevices.colPlatform')}</TableHead>
              <TableHead>{t('mobileDevices.colCreated')}</TableHead>
              <TableHead>{t('mobileDevices.colLastSeen')}</TableHead>
              <TableHead>{t('mobileDevices.colExpires')}</TableHead>
              <TableHead className="text-right">
                <span className="sr-only">{t('mobileDevices.revoke')}</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {devices.map((d) => (
              <TableRow key={d.id}>
                <TableCell className="font-medium">{d.name}</TableCell>
                <TableCell>{d.platform}</TableCell>
                <TableCell>{formatTime(d.created_at)}</TableCell>
                <TableCell>
                  {d.last_seen_at
                    ? formatTime(d.last_seen_at)
                    : t('mobileDevices.never')}
                </TableCell>
                <TableCell>
                  {d.expired
                    ? t('mobileDevices.expired')
                    : formatTime(d.expires_at)}
                </TableCell>
                <TableCell className="text-right">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => setTarget(d)}
                  >
                    {t('mobileDevices.revoke')}
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open) setTarget(null)
        }}
        title={t('mobileDevices.revokeTitle', { name: target?.name ?? '' })}
        description={t('mobileDevices.revokeDescription')}
        confirmLabel={t('mobileDevices.revoke')}
        tone="destructive"
        pending={revoke.isPending}
        error={revoke.error?.message ?? null}
        onConfirm={() => {
          if (!target) return
          revoke.mutate(target.id, {
            onSuccess: () => {
              toast.add({
                type: 'success',
                title: t('mobileDevices.revoked', { name: target.name }),
              })
              setTarget(null)
            },
          })
        }}
      />
      <StepUpDialog
        scope={gate.pending?.scope ?? null}
        onVerified={() => {
          void gate.onVerified()
        }}
        onCancel={gate.cancel}
      />
    </PageTemplate>
  )
}
