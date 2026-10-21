import { Suspense, lazy } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

const DomainDiagnosePanel = lazy(() =>
  import('./DomainDiagnosePanel').then((m) => ({
    default: m.DomainDiagnosePanel,
  })),
)

export function DomainDiagnoseDialog({
  domain,
  onClose,
}: {
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
      <DialogContent className="max-h-[85vh] overflow-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('diagnose.title', { domain })}</DialogTitle>
          <DialogDescription>{t('diagnose.description')}</DialogDescription>
        </DialogHeader>
        <Suspense fallback={null}>
          <DomainDiagnosePanel domain={domain} />
        </Suspense>
      </DialogContent>
    </Dialog>
  )
}
