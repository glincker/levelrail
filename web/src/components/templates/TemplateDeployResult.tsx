import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { ComposeDeployResponse } from '../../queries/compose'
import { TemplateAccessCard } from './TemplateAccessCard'

export interface TemplateDeployResultProps {
  result: ComposeDeployResponse | null
  templateId?: string
  onClose: () => void
}

/** Shown after a one-click template deploy so the generated logins are not lost. */
export function TemplateDeployResult({
  result,
  templateId,
  onClose,
}: TemplateDeployResultProps) {
  const { t } = useTranslation('templates')
  const main = result?.services[0]
  return (
    <Dialog
      open={result !== null && main !== undefined}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {t('result.title', { name: result?.app_id ?? '' })}
          </DialogTitle>
          <DialogDescription>
            {t('result.services', { count: result?.services.length ?? 0 })}
          </DialogDescription>
        </DialogHeader>
        {main ? (
          <TemplateAccessCard
            app={main}
            templateId={templateId}
            onNavigate={onClose}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
