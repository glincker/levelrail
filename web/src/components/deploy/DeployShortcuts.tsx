import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RocketGlassIcon, RotateCcwGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import type { GitDeployRef } from '../../queries/gitSourceCommits'

// The image tag is the last path segment, which keeps long registry hosts out of the UI.
function shortImage(image: string): string {
  const tail = image.split('/').pop() ?? image
  return tail.length > 40 ? tail.slice(0, 40) + '...' : tail
}

export function DeployShortcuts({
  last,
  previous,
  disabled,
  pending,
  onDeployImage,
}: {
  last?: GitDeployRef
  previous?: GitDeployRef
  disabled: boolean
  pending: boolean
  onDeployImage: (image: string) => void
}) {
  const { t } = useTranslation('smartDeploy')
  const [confirming, setConfirming] = useState(false)
  if (!last && !previous) return null
  return (
    <section aria-label={t('git.shortcuts.title')} className="space-y-2">
      <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
        {t('git.shortcuts.title')}
      </h3>
      <div className="flex flex-wrap gap-2">
        {last ? (
          <Button
            variant="outline"
            size="sm"
            disabled={disabled}
            title={t('git.shortcuts.redeployLastHint', {
              image: shortImage(last.image),
            })}
            onClick={() => {
              onDeployImage(last.image)
            }}
          >
            <RocketGlassIcon size="1.2em" aria-hidden="true" />
            {t('git.shortcuts.redeployLast')}
          </Button>
        ) : null}
        {previous ? (
          <Button
            variant="outline"
            size="sm"
            disabled={disabled}
            title={t('git.shortcuts.rollbackToHint', {
              image: shortImage(previous.image),
            })}
            onClick={() => {
              setConfirming(true)
            }}
          >
            <RotateCcwGlassIcon size="1.2em" aria-hidden="true" />
            {t('git.shortcuts.rollbackTo')}
          </Button>
        ) : null}
      </div>
      {previous ? (
        <ConfirmDialog
          open={confirming}
          onOpenChange={setConfirming}
          title={t('git.shortcuts.rollbackTitle', {
            image: shortImage(previous.image),
          })}
          description={t('git.shortcuts.rollbackDescription')}
          consequences={[t('git.shortcuts.rollbackConsequence')]}
          confirmLabel={t('git.shortcuts.rollbackConfirm')}
          pendingLabel={t('git.shortcuts.rollbackPending')}
          pending={pending}
          onConfirm={() => {
            onDeployImage(previous.image)
            setConfirming(false)
          }}
        />
      ) : null}
    </section>
  )
}
