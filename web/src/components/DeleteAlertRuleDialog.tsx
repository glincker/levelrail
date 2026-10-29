import { useTranslation } from 'react-i18next'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { toast } from '@/components/ui/toast'
import { useDeleteAlertRule } from '../queries/alerts'
import type { AlertRule } from '../types/alerts'

export function DeleteAlertRuleDialog({
  appName,
  rule,
}: {
  appName: string
  rule: AlertRule
}) {
  const { t } = useTranslation('common')
  const deleteRule = useDeleteAlertRule(appName)

  return (
    <ConfirmAction
      trigger={(openDialog) => (
        <Button variant="destructive" size="sm" onClick={openDialog}>
          <TrashGlassIcon className="size-5" aria-hidden="true" />
          {t('confirmAction.alertRule.trigger', { defaultValue: 'Delete' })}
        </Button>
      )}
      title={t('confirmAction.alertRule.title', {
        defaultValue: 'Delete "{{name}}"?',
        name: rule.name,
      })}
      consequences={[
        t('confirmAction.alertRule.consequence0', {
          defaultValue: 'The rule stops evaluating and notifying immediately.',
        }),
        t('confirmAction.alertRule.consequence1', {
          defaultValue: 'This cannot be undone.',
        }),
      ]}
      confirmLabel={t('confirmAction.alertRule.confirm', {
        defaultValue: 'Delete rule',
      })}
      pendingLabel={t('confirmAction.alertRule.pending', {
        defaultValue: 'Deleting...',
      })}
      pending={deleteRule.isPending}
      error={deleteRule.isError ? deleteRule.error.message : null}
      onOpenChange={(next) => {
        if (!next) deleteRule.reset()
      }}
      onConfirm={(close) => {
        deleteRule.mutate(rule.id, {
          onSuccess: () => {
            close()
            toast.add({
              title: t('confirmAction.alertRule.success', {
                defaultValue: 'Alert rule "{{name}}" deleted.',
                name: rule.name,
              }),
              type: 'success',
            })
          },
        })
      }}
    />
  )
}
