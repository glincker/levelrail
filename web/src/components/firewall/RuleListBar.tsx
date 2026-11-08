import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  PauseGlassIcon,
  TrashGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { toast } from '@/components/ui/toast'
import {
  useDeleteFirewallRule,
  useUpdateFirewallRule,
} from '../../queries/firewallEdge'
import { RULE_SORTS, type RuleSort } from './firewallListModel'
import { toRuleInput } from './firewallRuleModel'
import type { RuleList } from './useRuleList'

/** RuleListBar is the search box, sort menu, select-all box and bulk actions shared by the rule and IP list cards. */
export function RuleListBar({
  list,
  label,
}: {
  list: RuleList
  label: string
}) {
  const { t } = useTranslation('firewall')
  const update = useUpdateFirewallRule()
  const del = useDeleteFirewallRule()
  const [confirming, setConfirming] = useState(false)
  const [busy, setBusy] = useState(false)
  const count = list.selected.length

  function report(
    results: PromiseSettledResult<unknown>[],
    key: 'list.enabledCount' | 'list.disabledCount' | 'list.deletedCount',
  ) {
    const failed = results.filter((r) => r.status === 'rejected').length
    const done = results.length - failed
    if (done > 0) {
      toast.add({ title: t(key, { count: done }), type: 'success' })
    }
    if (failed > 0) {
      toast.add({
        title: t('list.bulkFailed', { count: failed }),
        type: 'error',
      })
    }
    list.clear()
  }

  async function setEnabled(enabled: boolean) {
    setBusy(true)
    const results = await Promise.allSettled(
      list.selected
        .filter((r) => r.enabled !== enabled)
        .map((r) =>
          update.mutateAsync({
            id: r.id,
            rule: { ...toRuleInput(r), enabled },
          }),
        ),
    )
    setBusy(false)
    report(results, enabled ? 'list.enabledCount' : 'list.disabledCount')
  }

  async function remove() {
    setBusy(true)
    const results = await Promise.allSettled(
      list.selected.map((r) => del.mutateAsync(r.id)),
    )
    setBusy(false)
    setConfirming(false)
    report(results, 'list.deletedCount')
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <Checkbox
          aria-label={t('list.selectAll')}
          checked={list.state === 'all'}
          indeterminate={list.state === 'some'}
          onCheckedChange={(on) => list.setAll(on)}
        />
        <Input
          type="search"
          aria-label={t('list.searchLabel', { list: label })}
          placeholder={t('list.searchPlaceholder')}
          className="min-w-40 flex-1 basis-48"
          value={list.query}
          onChange={(e) => list.setQuery(e.target.value)}
        />
        <NativeSelect
          aria-label={t('list.sortLabel')}
          value={list.sort}
          onChange={(e) => list.setSort(e.target.value as RuleSort)}
        >
          {RULE_SORTS.map((s) => (
            <option key={s} value={s}>
              {t(`list.sort.${s}`)}
            </option>
          ))}
        </NativeSelect>
      </div>
      {count > 0 ? (
        <div
          role="region"
          aria-label={t('list.barLabel')}
          className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/40 px-3 py-1.5"
        >
          <span className="text-sm font-medium" role="status">
            {t('list.selected', { count })}
          </span>
          <span className="ml-auto flex flex-wrap items-center gap-1.5">
            <Button
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={() => void setEnabled(true)}
            >
              <CircleCheckGlassIcon size="1.4em" aria-hidden="true" />
              {t('list.enable')}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={() => void setEnabled(false)}
            >
              <PauseGlassIcon size="1.4em" aria-hidden="true" />
              {t('list.disable')}
            </Button>
            <Button
              size="sm"
              variant="destructive"
              disabled={busy}
              onClick={() => setConfirming(true)}
            >
              <TrashGlassIcon size="1.4em" aria-hidden="true" />
              {t('list.delete')}
            </Button>
            <Button size="sm" variant="ghost" onClick={list.clear}>
              <XGlassIcon size="1.4em" aria-hidden="true" />
              {t('list.clear')}
            </Button>
          </span>
        </div>
      ) : null}
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('list.deleteTitle', { count })}
        description={t('list.deleteHelp')}
        confirmLabel={t('list.delete')}
        tone="destructive"
        pending={busy}
        onConfirm={() => void remove()}
      />
    </div>
  )
}
