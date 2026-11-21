import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { useScanSftpHostKey } from '../queries/backupTargets'

interface SftpHostKeyCheckProps {
  endpoint: string
  hostKey: string
  confirmed: boolean
  onHostKey: (key: string) => void
  onConfirmed: (confirmed: boolean) => void
}

// Pinning is an explicit two step action: fetch the key the server shows, then tick that its
// fingerprint was checked. Nothing is pinned silently and no credentials are sent for the fetch.
export function SftpHostKeyCheck({
  endpoint,
  hostKey,
  confirmed,
  onHostKey,
  onConfirmed,
}: SftpHostKeyCheckProps) {
  const { t } = useTranslation('backups')
  const scan = useScanSftpHostKey()
  const [keyType = '', fingerprint = ''] = hostKey.split(' ')

  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{t('sftpTarget.hostKeyTitle')}</p>
      <p className="text-xs text-muted-foreground">
        {t('sftpTarget.hostKeyHint')}
      </p>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={endpoint.trim() === '' || scan.isPending}
        onClick={() => {
          onConfirmed(false)
          scan.mutate(endpoint.trim(), {
            onSuccess: (res) => {
              onHostKey(res.host_key)
            },
          })
        }}
      >
        {scan.isPending ? t('sftpTarget.fetching') : t('sftpTarget.fetchKey')}
      </Button>
      {scan.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{scan.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {hostKey ? (
        <div className="space-y-2 rounded-md border p-3">
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
            <dt className="text-muted-foreground">{t('sftpTarget.keyType')}</dt>
            <dd className="font-mono">{keyType}</dd>
            <dt className="text-muted-foreground">
              {t('sftpTarget.fingerprint')}
            </dt>
            <dd className="font-mono break-all">{fingerprint}</dd>
          </dl>
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={confirmed}
              onCheckedChange={(checked) => {
                onConfirmed(checked === true)
              }}
            />
            {t('sftpTarget.confirm')}
          </label>
        </div>
      ) : null}
    </div>
  )
}
