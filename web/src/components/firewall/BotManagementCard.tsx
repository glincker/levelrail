import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { SettingRow, SettingRows } from '@/components/ui/setting-row'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { RuleSelect } from '../rules/RuleSelect'
import {
  useFirewallSettings,
  useSaveFirewallSettings,
  type FwMode,
  type FwSettings,
  type FwSettingsInput,
} from '../../queries/firewallEdge'
import { AiBotListDialog } from './AiBotListDialog'
import { LoadError } from './LoadError'
import { useFirewallLabels } from './useFirewallLabels'

const MODES: FwMode[] = ['off', 'log', 'challenge', 'block']

function input(s: FwSettings): FwSettingsInput {
  return {
    bot_mode: s.bot_mode,
    ai_bot_mode: s.ai_bot_mode,
    ai_bot_agents: s.ai_bot_agents,
    bans_enabled: s.bans_enabled,
  }
}

/** BotManagementCard sets how the curated bad bot and AI crawler lists are enforced, and automatic bans. */
export function BotManagementCard({ banNote }: { banNote?: string }) {
  const { t } = useTranslation('firewall')
  const labels = useFirewallLabels()
  const { data, error, refetch } = useFirewallSettings()
  const save = useSaveFirewallSettings()
  const [editing, setEditing] = useState(false)

  function apply(change: Partial<FwSettingsInput>, after?: () => void) {
    if (!data) return
    save.mutate(
      { ...input(data), ...change },
      {
        onSuccess: () => {
          toast.add({ title: t('bots.saved'), type: 'success' })
          after?.()
        },
        onError: (e) =>
          toast.add({
            title: t('actions.failed'),
            description: e.message,
            type: 'error',
          }),
      },
    )
  }

  const modeOptions = MODES.map((m) => ({ value: m, label: labels.mode[m] }))

  return (
    <Card className="min-w-0">
      <CardHeader className="border-b">
        <CardTitle>{t('bots.title')}</CardTitle>
        <CardDescription>{t('bots.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {error ? (
          <LoadError compact error={error} onRetry={() => void refetch()} />
        ) : !data ? (
          <div aria-hidden="true" className="space-y-4">
            {Array.from({ length: 3 }, (_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : (
          <SettingRows>
            <SettingRow
              title={t('bots.badBots')}
              description={t('bots.badBotsHelp')}
            >
              <RuleSelect
                aria-label={t('bots.badBots')}
                value={data.bot_mode}
                disabled={save.isPending}
                options={modeOptions}
                onChange={(e) => apply({ bot_mode: e.target.value as FwMode })}
              />
            </SettingRow>
            <SettingRow
              title={t('bots.aiBots')}
              description={t('bots.aiBotsHelp')}
            >
              <div className="flex flex-wrap items-center gap-2">
                <RuleSelect
                  aria-label={t('bots.aiBots')}
                  value={data.ai_bot_mode}
                  disabled={save.isPending}
                  options={modeOptions}
                  onChange={(e) =>
                    apply({ ai_bot_mode: e.target.value as FwMode })
                  }
                />
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setEditing(true)}
                >
                  {t('bots.editList')}
                </Button>
                <span className="text-xs text-muted-foreground">
                  {data.ai_bot_agents.length > 0
                    ? t('bots.customList', { count: data.ai_bot_agents.length })
                    : t('bots.shippedList', {
                        count: data.default_ai_bot_agents.length,
                      })}
                </span>
              </div>
            </SettingRow>
            <SettingRow
              title={t('bots.bans')}
              description={banNote ?? t('bots.bansHelp')}
            >
              <Switch
                checked={data.bans_enabled}
                disabled={save.isPending}
                aria-label={t('bots.bans')}
                onCheckedChange={(on) => apply({ bans_enabled: on })}
              />
            </SettingRow>
          </SettingRows>
        )}
        <p className="pt-3 text-xs text-muted-foreground">
          {t('bots.verifiedNote')}
        </p>
      </CardContent>
      {data && editing ? (
        <AiBotListDialog
          open
          onOpenChange={setEditing}
          agents={data.ai_bot_agents}
          defaults={data.default_ai_bot_agents}
          max={data.max_ai_bot_agents}
          pending={save.isPending}
          onSave={(agents) =>
            apply({ ai_bot_agents: agents }, () => setEditing(false))
          }
        />
      ) : null}
    </Card>
  )
}
