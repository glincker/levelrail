import { useState } from 'react'
import { TargetIcon } from '@phosphor-icons/react/dist/ssr'
import { Suggestion } from '@/components/kit'
import { toast } from '@/components/ui/toast'
import { useCreateAlertRule } from '../queries/alerts'
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
  const hasSlo = rules.some((r) => r.kind === 'slo_burn')
  const [dismissed, setDismissed] = useState(() => readDismissed(appName))
  const preview = useSloPreview(appName, DEFAULT_SLO, !hasSlo && !dismissed)
  const create = useCreateAlertRule(appName)

  if (hasSlo || dismissed || !preview.data?.has_traffic) return null

  return (
    <Suggestion
      tone="info"
      icon={<TargetIcon />}
      title={`Track a ${DEFAULT_SLO_TARGET}% availability SLO`}
      detail={`This app is serving traffic and has no SLO alert. A ${DEFAULT_SLO_TARGET}% target pages you when the 30 day error budget burns fast and opens a ticket-level alert when it burns slowly.`}
      actions={[
        {
          label: 'Create SLO rule',
          kind: 'primary',
          pending: create.isPending,
          onClick: async () => {
            try {
              await create.mutateAsync({
                name: `${DEFAULT_SLO_TARGET}% availability SLO`,
                kind: 'slo_burn',
                slo: DEFAULT_SLO,
                enabled: true,
              })
              toast.add({ title: 'SLO alert rule created.', type: 'success' })
            } catch (err) {
              toast.add({
                title:
                  err instanceof Error
                    ? err.message
                    : 'Could not create the SLO rule.',
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
