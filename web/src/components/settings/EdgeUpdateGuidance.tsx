import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ExternalLinkGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '../ui/alert'
import { Button } from '../ui/button'
import { toast } from '../ui/toast'
import {
  updateSettingsQueryOptions,
  useSetUpdateSettings,
  type EdgeGuidance,
  type UpdateChannel,
} from '../../queries/updates'
import { FixCli, type CliStep } from '../fixit/FixCli'

// What the edge channel means for this install. Edge is a moving container
// image, so a container gets the exact pull and recreate commands and a host
// binary is offered a one-click move to a channel that ships binaries.
export function EdgeUpdateGuidance({
  edge,
  updateAvailable,
}: {
  edge: EdgeGuidance
  updateAvailable: boolean
}) {
  const { t } = useTranslation('updates')
  const settings = useQuery(updateSettingsQueryOptions())
  const save = useSetUpdateSettings()

  const switchTo = (channel: UpdateChannel) => {
    save.mutate(
      {
        channel,
        auto_update_enabled: settings.data?.auto_update_enabled ?? true,
      },
      {
        onSuccess: () => {
          toast.add({
            title: t('edge.switched', {
              channel: t(`channels.${channel}.label`),
            }),
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({ title: error.message, type: 'error' })
        },
      },
    )
  }

  const edgeSteps: CliStep[] = []
  if (edge.pull_command) {
    edgeSteps.push({ label: t('edge.pull'), command: edge.pull_command })
  }
  if (edge.recreate_command) {
    edgeSteps.push({
      label: t('edge.recreate'),
      command: edge.recreate_command,
    })
  }
  if (edge.compose_snippet) {
    edgeSteps.push({ label: t('edge.compose'), command: edge.compose_snippet })
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">{t('edge.intro')}</p>
      <p className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        {edge.commit && edge.commit_url ? (
          <a
            href={edge.commit_url}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 font-mono text-primary hover:underline"
          >
            {t('edge.commit', { commit: edge.commit.slice(0, 7) })}
            <ExternalLinkGlassIcon className="size-4" />
          </a>
        ) : edge.commit ? (
          <span className="font-mono">
            {t('edge.commit', { commit: edge.commit.slice(0, 7) })}
          </span>
        ) : null}
        <span className="font-mono">{edge.image}</span>
      </p>
      {edge.installable ? (
        updateAvailable ? (
          <FixCli steps={edgeSteps} />
        ) : null
      ) : (
        <Alert>
          <AlertTitle>{t('edge.binaryTitle')}</AlertTitle>
          <AlertDescription className="space-y-2">
            <span className="block">{t('edge.binaryBody')}</span>
            <span className="flex flex-wrap gap-2">
              {(edge.switch_to ?? ['beta', 'stable']).map((c) => (
                <Button
                  key={c}
                  type="button"
                  size="sm"
                  variant={c === 'stable' ? 'default' : 'outline'}
                  disabled={save.isPending}
                  onClick={() => {
                    switchTo(c)
                  }}
                >
                  {t('edge.switchTo', { channel: t(`channels.${c}.label`) })}
                </Button>
              ))}
            </span>
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}
