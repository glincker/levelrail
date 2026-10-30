import { useTranslation } from 'react-i18next'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { MailQueuePanel } from './MailQueuePanel'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** Delivery queue with per message and bulk hold or release; opened from the header chip. */
export function MailQueueSheet({ open, onOpenChange }: Props) {
  const { t } = useTranslation('mailServer')
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="sm:max-w-3xl">
        <SheetHeader>
          <SheetTitle>{t('shell.queueTitle')}</SheetTitle>
        </SheetHeader>
        <div className="overflow-y-auto px-4 pb-4">
          <MailQueuePanel />
        </div>
      </SheetContent>
    </Sheet>
  )
}
