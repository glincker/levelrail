import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { PlusGlassIcon } from '@thesvg/ui-react'
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
  const { create, engine } = useSearch({ strict: false })
  const navigate = useNavigate()
  const [opened, setOpen] = useState(false)
  // An engine tile on the empty page sets ?create=1&engine=<id> to open us.
  const open = opened || create === '1'
  const initialEngine =
    create === '1' && typeof engine === 'string' && !engine.includes(',')
      ? engine
      : undefined
  const changeOpen = (next: boolean) => {
    setOpen(next)
    if (!next && create === '1') {
      void navigate({ to: '.', search: {}, replace: true })
    }
  }
  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger render={<Button type="button" />}>
        <PlusGlassIcon size="1.4em" aria-hidden="true" />
        {t('list.new')}
      </DialogTrigger>
      <DialogContent className="flex max-h-[90dvh] min-w-0 flex-col overflow-hidden sm:max-h-[90dvh] sm:max-w-2xl">
        <Suspense fallback={<PageSpinner />}>
          <DatabaseFlow
            initialEngine={initialEngine}
            onClose={() => {
              changeOpen(false)
            }}
          />
        </Suspense>
      </DialogContent>
    </Dialog>
  )
}
