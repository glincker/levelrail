import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import type { ParseKeys } from 'i18next'
import { useTranslation } from 'react-i18next'
import { LightbulbGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { toast } from '@/components/ui/toast'
import { Suggestion, type SuggestionAction } from '@/components/kit'
import {
  dismissSuggestion,
  dismissalKey,
  readDismissedSuggestions,
  visibleSuggestions,
} from '../../lib/deploySuggestionDismissal'
import { useSetPreviewEnabled } from '../../queries/previewEnvironments'
import type { DeploySuggestion } from '../../queries/gitSourceCommits'

function copyKey(
  s: DeploySuggestion,
  part: 'title' | 'detail' | 'action',
): ParseKeys<'smartDeploy'> {
  return `suggestions.${s.id}.${part}` as ParseKeys<'smartDeploy'>
}

export function DeploySuggestions({
  appName,
  suggestions,
  onNavigate,
}: {
  appName: string
  suggestions: DeploySuggestion[]
  onNavigate: () => void
}) {
  const { t } = useTranslation('smartDeploy')
  const navigate = useNavigate()
  const setPreview = useSetPreviewEnabled(appName)
  const [dismissed, setDismissed] = useState(() =>
    readDismissedSuggestions(appName),
  )
  const visible = visibleSuggestions(suggestions, dismissed)
  if (visible.length === 0) return null

  const actionFor = (s: DeploySuggestion): SuggestionAction => {
    const label = t(copyKey(s, 'action'))
    const go = (
      to: '/apps/$name/source' | '/apps/$name/health' | '/apps/$name/alerts',
    ) => {
      onNavigate()
      void navigate({ to, params: { name: appName } })
    }
    switch (s.action.type) {
      case 'enable_previews':
        return {
          label,
          pending: setPreview.isPending,
          onClick: () => {
            setPreview.mutate(true, {
              onSuccess: () => {
                toast.add({
                  title: t('suggestions.enable_previews.done'),
                  type: 'success',
                })
                setDismissed(dismissSuggestion(appName, dismissalKey(s)))
              },
              onError: (error) => {
                toast.add({ title: error.message, type: 'error' })
              },
            })
          },
        }
      case 'open_health_settings':
        return { label, onClick: () => go('/apps/$name/health') }
      case 'open_notifications':
        return { label, onClick: () => go('/apps/$name/alerts') }
      case 'open_deploy_logs':
        return {
          label,
          onClick: () => {
            const deployId = s.action.deploy_id
            if (!deployId) return
            onNavigate()
            void navigate({
              to: '/apps/$name/deploys/$deployId/logs',
              params: { name: appName, deployId },
            })
          },
        }
      default:
        return { label, onClick: () => go('/apps/$name/source') }
    }
  }

  return (
    <section aria-label={t('suggestions.title')} className="space-y-2">
      {visible.map((s) => (
        <Suggestion
          key={dismissalKey(s)}
          tone={s.severity === 'warning' ? 'warning' : 'info'}
          icon={
            s.severity === 'warning' ? (
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.4em"
                aria-hidden="true"
              />
            ) : (
              <LightbulbGlassIcon size="1.4em" aria-hidden="true" />
            )
          }
          title={t(copyKey(s, 'title'), s.params)}
          detail={t(copyKey(s, 'detail'), s.params)}
          actions={[actionFor(s)]}
          onDismiss={() => {
            setDismissed(dismissSuggestion(appName, dismissalKey(s)))
          }}
        />
      ))}
    </section>
  )
}
