import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RocketGlassIcon } from '@thesvg/ui-react'
import { Dialog, DialogContent, DialogTrigger } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { PageSpinner } from '@/components/ui/page-spinner'

const DeployFlow = lazy(() =>
  import('./deploy/DeployFlow').then((m) => ({ default: m.DeployFlow })),
)

export function DeployFunctionDialog() {
  const { t } = useTranslation('functions')
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button type="button" />}>
        <RocketGlassIcon size="1.4em" aria-hidden="true" />
        {t('deploy.open')}
      </DialogTrigger>
      <DialogContent className="flex max-h-[90dvh] min-w-0 flex-col overflow-hidden sm:max-h-[90dvh] sm:max-w-xl">
        <Suspense fallback={<PageSpinner />}>
          <DeployFlow
            onClose={() => {
              setOpen(false)
            }}
          />
        </Suspense>
      </DialogContent>
    </Dialog>
  )
}
