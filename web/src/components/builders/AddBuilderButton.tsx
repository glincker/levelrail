import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'

// The dialog chunk loads on first click so the nodes page stays light.
const AddBuilderDialog = lazy(() => import('./AddBuilderDialog'))

export function AddBuilderButton({
  variant = 'outline',
}: Readonly<{ variant?: 'outline' | 'default' }>) {
  const { t } = useTranslation('nodes')
  const [open, setOpen] = useState(false)
  const [loaded, setLoaded] = useState(false)
  return (
    <>
      <Button
        size="sm"
        variant={variant}
        onClick={() => {
          setLoaded(true)
          setOpen(true)
        }}
      >
        <PlusGlassIcon size="1.4em" />
        {t('builders.addButton')}
      </Button>
      {loaded ? (
        <Suspense fallback={null}>
          <AddBuilderDialog open={open} onOpenChange={setOpen} />
        </Suspense>
      ) : null}
    </>
  )
}
