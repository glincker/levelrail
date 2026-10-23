import { useTranslation } from 'react-i18next'
import { LockKeyIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'

export function EncryptedBackupBadge({ codec }: { codec?: string }) {
  const { t } = useTranslation('backups')
  if (!codec?.endsWith('+age')) {
    return <span className="text-muted-foreground">-</span>
  }
  return (
    <Badge variant="success" className="gap-1.5">
      <LockKeyIcon className="size-3.5" aria-hidden="true" />
      {t('encryption.badge')}
    </Badge>
  )
}
