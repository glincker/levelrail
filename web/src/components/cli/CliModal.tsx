import { lazy, Suspense } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { setCliModalOpen, useCliModalOpen } from '../../lib/cliModalStore'

const CliCommandCenter = lazy(() => import('./CliCommandCenter'))

// Mounted once in the app header so the shortcut and palette entry can open
// it even while the header badge is hidden. The body loads and fetches only
// while the dialog is open.
export function CliModal() {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const open = useCliModalOpen()
  return (
    <Dialog open={open} onOpenChange={setCliModalOpen}>
      <DialogContent className="sm:max-w-4xl">
        <Suspense
          fallback={
            <DialogHeader>
              <DialogTitle>{t('cliModal.title')}</DialogTitle>
              <DialogDescription>{t('cliModal.loadingBody')}</DialogDescription>
            </DialogHeader>
          }
        >
          <CliCommandCenter
            onFullPage={() => {
              setCliModalOpen(false)
            }}
          />
        </Suspense>
      </DialogContent>
    </Dialog>
  )
}
