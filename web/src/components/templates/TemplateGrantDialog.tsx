import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { TemplateCapability } from '../../queries/serviceTemplates'

/** Lists the elevated grants an install carries and asks for one explicit yes. */
export function TemplateGrantDialog({
  grants,
  open,
  pending,
  onOpenChange,
  onConfirm,
}: {
  grants: TemplateCapability[]
  open: boolean
  pending: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  const { t } = useTranslation('templates')
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('grantReview.title')}</DialogTitle>
          <DialogDescription>{t('grantReview.description')}</DialogDescription>
        </DialogHeader>
        <ul className="space-y-2 text-sm">
          {grants.map((g) => (
            <li
              key={`${g.service}-${g.class}`}
              className="space-y-0.5 rounded-md bg-muted px-3 py-2"
            >
              <p className="flex flex-wrap items-center gap-2">
                <Badge variant="destructive">
                  {t(`capabilities.tier.${g.tier}`)}
                </Badge>
                <span className="font-mono text-xs">
                  {g.service}
                  {g.caps?.length ? `: ${g.caps.join(', ')}` : ''}
                </span>
              </p>
              <p className="text-muted-foreground">
                {t(`capabilities.class.${g.class}`, { defaultValue: g.class })}
                {g.reason ? ` ${g.reason}` : ''}
              </p>
            </li>
          ))}
        </ul>
        <p className="text-xs text-muted-foreground">
          {t('grantReview.rootOnly')}
        </p>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => {
              onOpenChange(false)
            }}
          >
            {t('grantReview.cancel')}
          </Button>
          <Button disabled={pending} onClick={onConfirm}>
            {t('grantReview.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
