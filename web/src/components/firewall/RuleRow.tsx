import { useTranslation } from 'react-i18next'
import { EllipsisGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { ActionMenu, type ActionMenuItem } from '@/components/kit/ActionMenu'
import { Sparkline, StatusPill } from '@/components/kit'
import type { FwRule } from '../../queries/firewallEdge'
import { conditionText } from './firewallRuleModel'
import { ACTION_TONE, formatCount } from './firewallFormat'

/** RuleRow is one compact rule: position, name, what it matches, action, hits, an enabled switch and a menu. */
export function RuleRow({
  rule,
  actionLabel,
  menuItems,
  disabled,
  onToggle,
}: {
  rule: FwRule
  actionLabel: string
  menuItems: ActionMenuItem[]
  disabled?: boolean
  onToggle: (enabled: boolean) => void
}) {
  const { t } = useTranslation('firewall')
  const when = rule.conditions.map(conditionText).join(' AND ')
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-2 py-2 text-sm">
      <span className="w-6 shrink-0 text-xs text-muted-foreground tabular-nums">
        {rule.position}
      </span>
      <div className="min-w-0 flex-1 basis-48">
        <p className="truncate font-medium" title={rule.name}>
          {rule.name}
        </p>
        <p
          className="truncate font-mono text-xs text-muted-foreground"
          title={when}
        >
          {when}
        </p>
      </div>
      {rule.scope ? (
        <span
          className="max-w-40 truncate font-mono text-xs text-muted-foreground"
          title={rule.scope}
        >
          {rule.scope}
        </span>
      ) : null}
      <StatusPill
        tone={ACTION_TONE[rule.action]}
        label={actionLabel}
        size="sm"
      />
      <span
        className="w-12 text-right font-medium tabular-nums"
        title={t('rules.hitsTitle')}
      >
        {formatCount(rule.hits.total)}
      </span>
      <Sparkline
        values={rule.hits.sparkline}
        width={64}
        height={20}
        ariaLabel={t('rules.hitsSpark', { name: rule.name })}
      />
      <Switch
        size="sm"
        checked={rule.enabled}
        disabled={disabled}
        aria-label={t('rules.enabledFor', { name: rule.name })}
        onCheckedChange={onToggle}
      />
      <ActionMenu
        trigger={
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t('rules.actionsFor', { name: rule.name })}
          >
            <EllipsisGlassIcon size="1.2em" />
          </Button>
        }
        items={menuItems}
      />
    </div>
  )
}
