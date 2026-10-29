import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowDownGlassIcon,
  ArrowUpGlassIcon,
  ListChecksGlassIcon,
  PencilGlassIcon,
  PlusGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import type { ActionMenuItem } from '@/components/kit/ActionMenu'
import {
  useDeleteFirewallRule,
  useReorderFirewallRules,
  useUpdateFirewallRule,
  type FwRule,
} from '../../queries/firewallEdge'
import { movedOrder, toRuleInput } from './firewallRuleModel'
import { RuleRow } from './RuleRow'
import { VirtualRows } from './VirtualRows'
import { useFirewallLabels } from './useFirewallLabels'

/** CustomRulesCard lists the ordered custom rules; changes apply to traffic without a redeploy. */
export function CustomRulesCard({
  all,
  custom,
  onNew,
  onEdit,
}: {
  all: FwRule[]
  custom: FwRule[]
  onNew: () => void
  onEdit: (id: string) => void
}) {
  const { t } = useTranslation('firewall')
  const labels = useFirewallLabels()
  const update = useUpdateFirewallRule()
  const reorder = useReorderFirewallRules()
  const del = useDeleteFirewallRule()
  const [removing, setRemoving] = useState<FwRule | null>(null)

  function fail(e: Error) {
    toast.add({
      title: t('actions.failed'),
      description: e.message,
      type: 'error',
    })
  }

  function toggle(rule: FwRule, enabled: boolean) {
    update.mutate(
      { id: rule.id, rule: { ...toRuleInput(rule), enabled } },
      { onError: fail },
    )
  }

  function move(rule: FwRule, dir: -1 | 1) {
    const ids = movedOrder(all, custom, rule.id, dir)
    if (ids) reorder.mutate(ids, { onError: fail })
  }

  function menu(rule: FwRule, index: number): ActionMenuItem[] {
    return [
      {
        id: 'edit',
        label: t('rules.edit'),
        icon: <PencilGlassIcon size="1.4em" />,
        onSelect: () => onEdit(rule.id),
      },
      {
        id: 'up',
        label: t('rules.moveUp'),
        icon: <ArrowUpGlassIcon size="1.4em" />,
        disabled: index === 0,
        onSelect: () => move(rule, -1),
      },
      {
        id: 'down',
        label: t('rules.moveDown'),
        icon: <ArrowDownGlassIcon size="1.4em" />,
        disabled: index === custom.length - 1,
        onSelect: () => move(rule, 1),
      },
      {
        id: 'delete',
        label: t('rules.delete'),
        icon: <TrashGlassIcon size="1.4em" />,
        tone: 'danger',
        onSelect: () => setRemoving(rule),
      },
    ]
  }

  return (
    <Card className="min-w-0">
      <CardHeader className="border-b">
        <CardTitle>{t('rules.customTitle')}</CardTitle>
        <CardDescription>{t('rules.customHelp')}</CardDescription>
        <div data-slot="card-action">
          <Button type="button" size="sm" onClick={onNew}>
            <PlusGlassIcon size="1.4em" />
            {t('rules.new')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="px-2">
        {custom.length === 0 ? (
          <EmptyState
            compact
            icon={<ListChecksGlassIcon className="size-6" />}
            title={t('rules.emptyTitle')}
            description={t('rules.emptyHelp')}
            action={
              <Button type="button" size="sm" onClick={onNew}>
                {t('rules.new')}
              </Button>
            }
          />
        ) : (
          <VirtualRows
            items={custom}
            rowHeight={56}
            label={t('rules.customTitle')}
            getKey={(r) => r.id}
            renderRow={(r, i) => (
              <RuleRow
                rule={r}
                actionLabel={labels.action[r.action]}
                menuItems={menu(r, i)}
                disabled={update.isPending}
                onToggle={(enabled) => toggle(r, enabled)}
              />
            )}
          />
        )}
      </CardContent>
      <ConfirmDialog
        open={removing !== null}
        onOpenChange={(o) => {
          if (!o) setRemoving(null)
        }}
        title={t('rules.deleteTitle', { name: removing?.name ?? '' })}
        description={t('rules.deleteHelp')}
        confirmLabel={t('rules.delete')}
        tone="destructive"
        pending={del.isPending}
        onConfirm={() => {
          if (removing)
            del.mutate(removing.id, {
              onSuccess: () => setRemoving(null),
              onError: fail,
            })
        }}
      />
    </Card>
  )
}
