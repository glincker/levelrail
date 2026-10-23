import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { TemplatePicker } from '@/components/dashboards/TemplatePicker'
import type { DashboardDoc } from '@/types/dashboards'

/** Pick a starter template, or start from an empty dashboard. */
export function NewDashboardDialog({
  open,
  busy,
  emptyName,
  onOpenChange,
  onCreate,
}: {
  open: boolean
  busy: boolean
  emptyName: string
  onOpenChange: (open: boolean) => void
  onCreate: (doc: DashboardDoc) => void
}) {
  const { t } = useTranslation('dashboard')
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('tabs.addTitle')}</DialogTitle>
          <DialogDescription>{t('tabs.addDescription')}</DialogDescription>
        </DialogHeader>
        <TemplatePicker busy={busy} onPick={onCreate} />
        <div>
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => {
              onCreate({ version: 1, name: emptyName, widgets: [] })
            }}
          >
            {t('tabs.empty')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
