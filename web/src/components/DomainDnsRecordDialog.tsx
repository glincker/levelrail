import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { DomainDnsCheck } from './DomainDnsCheck'

export function DomainDnsRecordDialog({
  appName,
  domain,
  onClose,
}: {
  appName: string
  domain: string
  onClose: () => void
}) {
  const { t } = useTranslation('domains')
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('page.dnsDialog.title', { domain })}</DialogTitle>
        </DialogHeader>
        <DomainDnsCheck appName={appName} domain={domain} />
      </DialogContent>
    </Dialog>
  )
}
