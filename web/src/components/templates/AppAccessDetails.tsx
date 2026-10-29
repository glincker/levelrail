import { useTranslation } from 'react-i18next'
import { KeyGlassIcon } from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { AppDetail } from '../../types/appDetail'
import { TemplateAccessCard } from './TemplateAccessCard'
import { hasTemplateAccess } from './templateAccess'

/** Compact overview card so generated logins stay reachable after the deploy dialog is gone. */
export function AppAccessDetails({ app }: { app: AppDetail }) {
  const { t } = useTranslation('templates')
  if (!hasTemplateAccess(app)) return null
  return (
    <section
      id="access-details"
      aria-labelledby="access-details-title"
      className="min-w-0 rounded-lg border bg-card p-4"
    >
      <h2
        id="access-details-title"
        className="mb-2 flex items-center gap-1.5 text-sm font-semibold"
      >
        <KeyGlassIcon className="size-4.5" aria-hidden="true" />
        {t('access.title')}
      </h2>
      <TemplateAccessCard app={app} hideLinks />
    </section>
  )
}

export function AppAccessDetailsDialog({
  app,
  open,
  onOpenChange,
}: {
  app: AppDetail
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('templates')
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('access.title')}</DialogTitle>
          <DialogDescription>{t('access.description')}</DialogDescription>
        </DialogHeader>
        {open ? <TemplateAccessCard app={app} hideLinks /> : null}
      </DialogContent>
    </Dialog>
  )
}
