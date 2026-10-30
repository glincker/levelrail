import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  CopyGlassIcon,
  LockGlassIcon,
  LockOpenGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { toast } from '@/components/ui/toast'
import { useAckBackupKey } from '../../queries/backupCoverage'
import {
  useBackupHealth,
  useRefreshBackupProtection,
} from '../../queries/backupProtection'
import type { CoverageEncryption } from '../../types/backupCoverage'

function useCopyPublicKey(recipient: string | undefined) {
  const { t } = useTranslation('backups')
  return () => {
    if (!recipient) return
    navigator.clipboard.writeText(recipient).then(
      () => {
        toast.add({ title: t('safety.copied'), type: 'success' })
      },
      () => {
        toast.add({ title: t('safety.copyFailed'), type: 'error' })
      },
    )
  }
}

function KeyDisclosure({ encryption }: { encryption: CoverageEncryption }) {
  const { t } = useTranslation('backups')
  const copy = useCopyPublicKey(encryption.recipient)
  return (
    <Collapsible>
      <CollapsibleTrigger className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
        <ChevronDownGlassIcon className="size-5" aria-hidden="true" />
        {t('safety.keyDetails')}
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <div className="mt-2 space-y-2 text-sm text-muted-foreground">
          <p>{t('safety.keyHint')}</p>
          <Button type="button" size="sm" variant="outline" onClick={copy}>
            <CopyGlassIcon className="size-5" aria-hidden="true" />
            {t('safety.copyKey')}
          </Button>
        </div>
      </CollapsiblePanel>
    </Collapsible>
  )
}

function KeyWarning() {
  const { t } = useTranslation('backups')
  const ack = useAckBackupKey()
  const [hidden, setHidden] = useState(false)
  if (hidden) return null
  return (
    <Alert>
      <TriangleAlertGlassIcon
        className="text-tone-warning"
        size="1.7em"
        aria-hidden="true"
      />
      <AlertDescription className="space-y-2">
        <p>{t('safety.keyNotStored')}</p>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            disabled={ack.isPending}
            onClick={() => {
              ack.mutate(undefined, {
                onError: (err) => {
                  toast.add({
                    title: t('safety.ackFailed'),
                    description: err.message,
                    type: 'error',
                  })
                },
              })
            }}
          >
            {t('safety.ack')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => {
              setHidden(true)
            }}
          >
            {t('safety.later')}
          </Button>
        </div>
      </AlertDescription>
    </Alert>
  )
}

/** StorageSafetyCard is the compact encryption and bucket protection status of backups. */
export function StorageSafetyCard({
  encryption,
}: {
  encryption: CoverageEncryption
}) {
  const { t } = useTranslation('backups')
  const health = useBackupHealth()
  const refresh = useRefreshBackupProtection()
  const warnings = (health.data?.targets ?? []).filter(
    (x) => x.warning || x.probe_error,
  )

  return (
    <section
      className="space-y-3 rounded-lg border border-border p-4"
      aria-labelledby="storage-safety-title"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="storage-safety-title" className="text-base font-medium">
          {t('safety.title')}
        </h2>
        <div className="flex flex-wrap items-center gap-2">
          {encryption.enabled ? (
            <Badge variant="success">
              <LockGlassIcon className="size-4.5" aria-hidden="true" />
              {t('safety.encrypted')}
            </Badge>
          ) : (
            <Badge variant="warning">
              <LockOpenGlassIcon className="size-4.5" aria-hidden="true" />
              {t('safety.notEncrypted')}
            </Badge>
          )}
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={refresh.isPending}
            onClick={() => {
              refresh.mutate(undefined, {
                onError: (err) => {
                  toast.add({
                    title: t('protection.checkFailed'),
                    description: err.message,
                    type: 'error',
                  })
                },
              })
            }}
          >
            {refresh.isPending
              ? t('protection.checking')
              : t('protection.check')}
          </Button>
        </div>
      </div>

      {!encryption.enabled ? (
        <p className="text-sm text-muted-foreground">{t('encryption.off')}</p>
      ) : (
        <>
          {!encryption.key_stored_off_box ? <KeyWarning /> : null}
          <KeyDisclosure encryption={encryption} />
        </>
      )}

      {warnings.map((w) => (
        <Alert key={w.target_id} variant="destructive">
          <TriangleAlertGlassIcon
            className="text-tone-warning"
            size="1.7em"
            aria-hidden="true"
          />
          <AlertDescription>
            <strong>{t('protection.target', { id: w.target_id })}</strong>{' '}
            {w.probe_error
              ? t('protection.probeError', { error: w.probe_error })
              : w.warning}
          </AlertDescription>
        </Alert>
      ))}
    </section>
  )
}
