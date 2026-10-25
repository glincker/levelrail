import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearch } from '@tanstack/react-router'
import { PlusIcon } from '@phosphor-icons/react/dist/ssr'
import { Dialog, DialogContent, DialogTrigger } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { PageSpinner } from '@/components/ui/page-spinner'

const DatabaseFlow = lazy(() =>
  import('./databaseFlow/DatabaseFlow').then((m) => ({
    default: m.DatabaseFlow,
  })),
)

export function CreateDatabaseDialog() {
  const { t } = useTranslation('databases')
  // /databases/new redirects here with ?create=1 so the link opens the flow.
  const { create } = useSearch({ strict: false })
  const [open, setOpen] = useState(create === '1')
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button type="button" />}>
        <PlusIcon aria-hidden="true" />
        {t('list.new')}
      </DialogTrigger>
      <DialogContent className="flex max-h-[90dvh] min-w-0 flex-col overflow-hidden sm:max-h-[90dvh] sm:max-w-2xl">
        <Suspense fallback={<PageSpinner />}>
          <DatabaseFlow
            onClose={() => {
              setOpen(false)
            }}
          />
        </Suspense>
      </DialogContent>
    </Dialog>
  )
}
