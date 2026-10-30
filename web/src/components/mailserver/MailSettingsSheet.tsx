import { useTranslation } from 'react-i18next'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import type { MailDomain } from '../../queries/mailMailboxes'
import { MailStoragePanel } from './MailStoragePanel'
import { MailSystemEmailCard } from './MailSystemEmailCard'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  domains: MailDomain[]
  app: string | undefined
}

/** Settings sheet: platform emails and backup destination, kept out of the main views. */
export function MailSettingsSheet({ open, onOpenChange, domains, app }: Props) {
  const { t } = useTranslation('mailServer')
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>{t('shell.settingsTitle')}</SheetTitle>
        </SheetHeader>
        <div className="flex flex-col gap-3 overflow-y-auto px-4 pb-4">
          <MailSystemEmailCard domains={domains} />
          {app ? <MailStoragePanel app={app} /> : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}
