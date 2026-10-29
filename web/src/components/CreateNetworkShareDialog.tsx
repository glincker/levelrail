import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Dialog, DialogContent, DialogTrigger } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { PageSpinner } from '@/components/ui/page-spinner'

const ShareFlow = lazy(() =>
  import('./networkShares/ShareFlow').then((m) => ({ default: m.ShareFlow })),
)

// POST /api/v1/network-shares needs write:sensitive for an SMB share because
// the password travels in the request body.
export function CreateNetworkShareDialog() {
  const { t } = useTranslation('settings')
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button type="button" />}>
        <PlusGlassIcon size="1.4em" aria-hidden="true" />
        {t('networkSharesPage.dialog.open')}
      </DialogTrigger>
      <DialogContent className="flex max-h-[90dvh] min-w-0 flex-col overflow-hidden sm:max-h-[90dvh] sm:max-w-xl">
        <Suspense fallback={<PageSpinner />}>
          <ShareFlow
            onClose={() => {
              setOpen(false)
            }}
          />
        </Suspense>
      </DialogContent>
    </Dialog>
  )
}
