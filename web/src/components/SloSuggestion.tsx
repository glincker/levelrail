import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TargetGlassIcon } from '@thesvg/ui-react'
import { Suggestion } from '@/components/kit'
import { toast } from '@/components/ui/toast'
import { useCreateAlertRule } from '../queries/alerts'
import { useNotificationChannelsOptional } from '../queries/notificationChannels'
import { DEFAULT_SLO_TARGET, useSloPreview } from '../queries/sloPreview'
import type { AlertRule, SloConfig } from '../types/alerts'

const DEFAULT_SLO: SloConfig = {
  objective: 'availability',
  target: DEFAULT_SLO_TARGET,
}

function dismissKey(app: string): string {
  return `slo-suggestion-dismissed:${app}`
}

function readDismissed(app: string): boolean {
  try {
    return window.localStorage.getItem(dismissKey(app)) === '1'
  } catch {
    return false
  }
}

// Offers a default availability SLO to an app that receives traffic but has
// no slo_burn rule. Renders nothing otherwise.
export function SloSuggestion({
  appName,
  rules,
}: {
  appName: string
  rules: AlertRule[]
}) {
  const { t } = useTranslation('alertSetup')
  const hasSlo = rules.some((r) => r.kind === 'slo_burn')
  const [dismissed, setDismissed] = useState(() => readDismissed(appName))
  const preview = useSloPreview(appName, DEFAULT_SLO, !hasSlo && !dismissed)
  const create = useCreateAlertRule(appName)
  const channels = useNotificationChannelsOptional()
  const channelId =
    rules.find((r) => r.channel_id)?.channel_id ?? channels.data?.[0]?.id

  if (hasSlo || dismissed || !preview.data?.has_traffic) return null

  return (
    <Suggestion
      tone="info"
      icon={<TargetGlassIcon size="1.4em" />}
      title={t('slo.title', { target: DEFAULT_SLO_TARGET })}
      detail={`${t('slo.detail', { target: DEFAULT_SLO_TARGET })}${
        channelId ? '' : ` ${t('slo.noChannel')}`
      }`}
      actions={[
        {
          label: t('slo.create'),
          kind: 'primary',
          pending: create.isPending,
          onClick: async () => {
            try {
              await create.mutateAsync({
                name: t('slo.ruleName', { target: DEFAULT_SLO_TARGET }),
                kind: 'slo_burn',
                slo: DEFAULT_SLO,
                channel_id: channelId,
                enabled: true,
              })
              toast.add({ title: t('slo.created'), type: 'success' })
            } catch (err) {
              toast.add({
                title:
                  err instanceof Error ? err.message : t('slo.createFailed'),
                type: 'error',
              })
            }
          },
        },
      ]}
      onDismiss={() => {
        setDismissed(true)
        try {
          window.localStorage.setItem(dismissKey(appName), '1')
        } catch {
          // Dismissal just will not persist.
        }
      }}
    />
  )
}
