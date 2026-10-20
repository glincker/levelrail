import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckIcon,
  CopyIcon,
  SealCheckIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { InfoTip } from '@/components/kit'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useBadgeSetting, useSetBadgeSetting } from '../queries/badge'

// Matches the copy-row idiom GitSourceWebhookBanner/cli-access.tsx
// already established (code block + copy button in a muted bordered
// row); kept local for the same "no other page shows this exact shape"
// reason cli-access.tsx's own CopyCommand gives.
function CopySnippet({ text }: { text: string }) {
  const { t } = useTranslation('common')
  const [copied, setCopied] = useState(false)
  return (
    <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
      <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
        {text}
      </code>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => {
          void navigator.clipboard.writeText(text).then(() => {
            setCopied(true)
            setTimeout(() => {
              setCopied(false)
            }, 2000)
          })
        }}
      >
        {copied ? (
          <CheckIcon aria-hidden="true" />
        ) : (
          <CopyIcon aria-hidden="true" />
        )}
        {copied ? t('actions.copied') : t('actions.copy')}
      </Button>
    </div>
  )
}

// BadgeSettingsCard is the opt-in toggle for the public deploy status
// badge (GET/PUT /api/v1/apps/{name}/badge, internal/api/app_badge.go):
// off by default, the opposite shape ExecAccessCard's own opt-out
// toggle establishes, since enabling it exposes this app's deploy
// status to anyone with the badge.svg URL. Rendered on the deploys
// index page alongside AutoRollbackCard/AutoRollbackSLOBurnCard, the
// other opt-in per-app deploy settings.
export function BadgeSettingsCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const setting = useBadgeSetting(appName)
  const setBadge = useSetBadgeSetting(appName)
  const badgeUrl = `${window.location.origin}/api/v1/apps/${encodeURIComponent(appName)}/badge.svg`

  function toggle(next: boolean) {
    setBadge.mutate(next, {
      onSuccess: () => {
        toast.add({
          title: next
            ? t('badgeSettings.toast.enabled')
            : t('badgeSettings.toast.disabled'),
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: t('badgeSettings.toast.errorTitle'),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <SealCheckIcon className="size-4 text-muted-foreground" />
          {t('badgeSettings.title')}
          <InfoTip label={t('badgeSettings.infoTipLabel')}>
            {t('badgeSettings.description')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between gap-4">
          <p className="text-sm font-medium text-foreground">
            {t('badgeSettings.enabledLabel')}
          </p>
          <Switch
            checked={setting.data.enabled}
            onCheckedChange={toggle}
            disabled={setBadge.isPending}
            aria-label={t('badgeSettings.switchAriaLabel')}
          />
        </div>

        {setting.data.enabled && (
          <div className="space-y-2">
            <p className="text-sm font-medium text-foreground">
              {t('badgeSettings.embedLabel')}
            </p>
            <CopySnippet text={`![deploy status](${badgeUrl})`} />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
