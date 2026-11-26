import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CloudGlassIcon } from '@thesvg/ui-react'
import { Dialog, DialogContent, DialogTrigger } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { PageSpinner } from '@/components/ui/page-spinner'

// The flow loads on first open so the Databases page bundle stays small.
const HostedConnectFlow = lazy(() =>
  import('./hostedConnect/HostedConnectFlow').then((m) => ({
    default: m.HostedConnectFlow,
  })),
)

export function ConnectHostedDialog({
  compact = false,
}: Readonly<{ compact?: boolean }>) {
  const { t } = useTranslation('databases')
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger
        render={
          <Button
            type="button"
            size={compact ? 'xs' : 'sm'}
            variant="outline"
          />
        }
      >
        {compact ? null : (
          <CloudGlassIcon className="size-5" aria-hidden="true" />
        )}
        {t(compact ? 'where.hostedAction' : 'external.hosted.trigger')}
      </DialogTrigger>
      <DialogContent className="flex max-h-[90dvh] min-w-0 flex-col overflow-hidden sm:max-h-[90dvh] sm:max-w-2xl">
        <Suspense fallback={<PageSpinner />}>
          <HostedConnectFlow
            onClose={() => {
              setOpen(false)
            }}
          />
        </Suspense>
      </DialogContent>
    </Dialog>
  )
}
