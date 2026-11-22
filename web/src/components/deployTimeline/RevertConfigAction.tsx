import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { UndoGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  useRevertConfig,
  type ConfigFieldChange,
  type RevertResult,
} from '../../queries/appConfigRevisions'

const DASH = '-'

function line(c: ConfigFieldChange): string {
  return `${c.kind} ${c.field}: ${c.from || DASH} > ${c.to || DASH}`
}

// "Revert config to here" on a history entry: previews the diff with a dry
// run, then applies it as a new revision after a confirm.
export function RevertConfigAction({
  appName,
  revision,
}: Readonly<{ appName: string; revision: number }>) {
  const { t } = useTranslation('deployTimeline')
  const revert = useRevertConfig(appName)
  const [plan, setPlan] = useState<RevertResult | null>(null)

  const preview = () => {
    revert.mutate(
      { to: revision, dryRun: true },
      {
        onSuccess: (res) => {
          setPlan(res)
        },
      },
    )
  }
  const lines = plan
    ? plan.changes.length === 0
      ? [t('revertConfig.none')]
      : plan.changes.map(line)
    : []

  return (
    <>
      <Button
        variant="ghost"
        size="xs"
        onClick={preview}
        disabled={revert.isPending}
      >
        <UndoGlassIcon className="size-5" data-icon="inline-start" />
        {t('revertConfig.action')}
      </Button>
      <ConfirmDialog
        open={plan !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPlan(null)
            revert.reset()
          }
        }}
        title={t('revertConfig.title', { revision })}
        description={t('revertConfig.disclaimer')}
        consequences={lines}
        confirmLabel={t('revertConfig.confirm')}
        pendingLabel={t('revertConfig.pending')}
        pending={revert.isPending}
        confirmDisabled={(plan?.changes.length ?? 0) === 0}
        error={revert.error?.message ?? null}
        onConfirm={() => {
          revert.mutate(
            { to: revision, dryRun: false },
            {
              onSuccess: () => {
                setPlan(null)
              },
            },
          )
        }}
      />
    </>
  )
}
