import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { ShieldAlertGlassIcon } from '@thesvg/ui-react'
import { Checkbox } from '@/components/ui/checkbox'
import { TONE } from '@/components/kit/tone'
import { cn } from '@/lib/utils'

export function ProtectedDeployNotice({
  environmentName,
  acknowledged,
  onAcknowledgedChange,
}: {
  environmentName: string
  acknowledged: boolean
  onAcknowledgedChange: (next: boolean) => void
}) {
  const { t } = useTranslation('smartDeploy')
  const id = useId()
  return (
    <div
      className={cn(
        'space-y-2 rounded-lg border p-3 text-sm',
        TONE.warning.soft,
        TONE.warning.border,
      )}
    >
      <div className="flex items-start gap-2">
        <ShieldAlertGlassIcon
          className={cn('mt-0.5 size-5 shrink-0', TONE.warning.text)}
          aria-hidden="true"
        />
        <p>{t('approval.protected', { environment: environmentName })}</p>
      </div>
      <label htmlFor={id} className="flex items-center gap-2 pl-6">
        <Checkbox
          id={id}
          checked={acknowledged}
          onCheckedChange={(checked) => {
            onAcknowledgedChange(checked === true)
          }}
        />
        {t('approval.ack')}
      </label>
    </div>
  )
}
