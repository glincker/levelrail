import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import type { DatabaseResource } from '../../types/databaseDetail'

// Lazy so the dialog and its snippet generators stay out of the first load.
const ConnectDialog = lazy(() =>
  import('./ConnectDialog').then((m) => ({ default: m.ConnectDialog })),
)

/** The green Connect button; the dialog loads on first open. */
export function ConnectDrawer({ database }: { database: DatabaseResource }) {
  const { t } = useTranslation('databases')
  const [open, setOpen] = useState(false)
  const [opened, setOpened] = useState(false)

  return (
    <>
      <Button
        size="sm"
        className="bg-(--glinr-status-success-solid) text-white hover:bg-(--glinr-status-success-solid) hover:brightness-95"
        onClick={() => {
          setOpened(true)
          setOpen(true)
        }}
        aria-haspopup="dialog"
      >
        <PlugGlassIcon size="1.4em" aria-hidden="true" />
        {t('connectDrawer.trigger')}
      </Button>
      {opened ? (
        <Suspense fallback={null}>
          <ConnectDialog
            database={database}
            open={open}
            onOpenChange={setOpen}
          />
        </Suspense>
      ) : null}
    </>
  )
}
