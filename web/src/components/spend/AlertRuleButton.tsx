import { useTranslation } from 'react-i18next'
import { BellRingGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useAlertRules, useCreateAlertRule } from '../../queries/alerts'

/** AlertRuleButton creates the budget_threshold rule when none exists, so cap alerts reach a channel. */
export function AlertRuleButton({ appName }: { appName: string }) {
  const { t } = useTranslation('budget')
  const rules = useAlertRules(appName)
  const create = useCreateAlertRule(appName)
  if (!rules.data || rules.data.some((r) => r.kind === 'budget_threshold')) {
    return null
  }
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      disabled={create.isPending}
      onClick={() =>
        create.mutate(
          {
            name: t('alertRule.name'),
            kind: 'budget_threshold',
            enabled: true,
          },
          {
            onSuccess: () =>
              toast.add({ title: t('alertRule.created'), type: 'success' }),
            onError: (e) =>
              toast.add({
                title: t('toast.errorTitle'),
                description: e.message,
                type: 'error',
              }),
          },
        )
      }
    >
      <BellRingGlassIcon size="1.2em" aria-hidden="true" />
      {create.isPending ? t('alertRule.creating') : t('alertRule.create')}
    </Button>
  )
}
