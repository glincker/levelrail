import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { WandGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useApplyAlertTemplate, type AlertProfile } from '../queries/alertSetup'
import { useNotificationChannelsOptional } from '../queries/notificationChannels'

const PROFILES: AlertProfile[] = ['web', 'worker', 'database']

// One click creates a sensible rule set for the app type. Rules that already
// exist by name are skipped server side, so it is safe to press twice.
export function AlertTemplatePicker({
  appName,
  channelId,
}: {
  appName: string
  channelId?: string
}) {
  const { t } = useTranslation('alertSetup')
  const [profile, setProfile] = useState<AlertProfile>('web')
  const apply = useApplyAlertTemplate(appName)
  const channels = useNotificationChannelsOptional()
  const channel = channelId ?? channels.data?.[0]?.id

  async function onApply() {
    try {
      const res = await apply.mutateAsync({ profile, channel_id: channel })
      const count = res.created.length
      const skipped = res.skipped.length
      toast.add({
        title:
          count === 0
            ? t('template.allExist')
            : skipped > 0
              ? t('template.createdSkipped', { count, skipped })
              : t('template.created', { count }),
        description: channel ? undefined : t('template.noChannel'),
        type: 'success',
      })
    } catch (err) {
      toast.add({
        title: err instanceof Error ? err.message : t('template.failed'),
        type: 'error',
      })
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Select
        value={profile}
        onValueChange={(v) => setProfile(v as AlertProfile)}
      >
        <SelectTrigger aria-label={t('template.profile')} className="w-36">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {PROFILES.map((p) => (
            <SelectItem key={p} value={p}>
              {t(`template.${p}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        type="button"
        variant="outline"
        disabled={apply.isPending}
        onClick={() => void onApply()}
        title={t(`template.${profile}Hint`)}
      >
        <WandGlassIcon size="1.2em" />
        {apply.isPending ? t('template.applying') : t('template.apply')}
      </Button>
    </div>
  )
}
