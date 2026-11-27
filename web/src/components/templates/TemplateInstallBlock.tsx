import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Checkbox } from '@/components/ui/checkbox'
import type { InstallBlock } from '../../lib/templatePlatform'

/** Names why an install was refused and what unlocks it; the arch case offers the override. */
export function TemplateInstallBlock({
  block,
  message,
  allowMismatch,
  onAllowMismatch,
}: {
  block: InstallBlock
  message: string
  allowMismatch: boolean
  onAllowMismatch: (v: boolean) => void
}) {
  const { t } = useTranslation('templates')
  const id = useId()
  return (
    <Alert variant="destructive">
      <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
      <AlertDescription className="space-y-2">
        <p className="font-medium">{t(`installBlock.${block.kind}.title`)}</p>
        <p>{message}</p>
        <p>
          {t(`installBlock.${block.kind}.fix`, {
            module: block.kind === 'kernel' ? block.module : '',
          })}
        </p>
        {block.kind === 'arch' ? (
          <div className="flex items-center gap-2">
            <Checkbox
              id={id}
              checked={allowMismatch}
              onCheckedChange={onAllowMismatch}
            />
            <label htmlFor={id} className="text-sm">
              {t('installBlock.arch.override')}
            </label>
          </div>
        ) : null}
      </AlertDescription>
    </Alert>
  )
}
