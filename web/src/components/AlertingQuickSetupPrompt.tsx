import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { BellRingingIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertAction, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useCreateAlertRule } from '../queries/alerts'
import { useNotificationChannelsOptional } from '../queries/notificationChannels'
import { CreateNotificationChannelDialog } from './CreateNotificationChannelDialog'
import { CHANNEL_KIND_LABEL } from './notificationChannelKind'
import type { AlertRuleKind, CreateAlertRuleRequest } from '../types/alerts'

const DISMISSED_STORAGE_KEY = 'dashboard-alerting-quick-setup-dismissed'

// The four platform-wide kinds (internal/alerting/rules.go's Kind enum):
// each watches every certificate/node on the whole control plane, not a
// single app, so ResourceID is only ever a display label for them (see
// rules.go's own doc comment), and the name below is all that
// distinguishes one from another in the list this creates.
const RECOMMENDED_RULES: { kind: AlertRuleKind; name: string }[] = [
  { kind: 'cert_expiry', name: 'Certificate expiry' },
  { kind: 'patch_status', name: 'Node patch status' },
  { kind: 'node_disk_space', name: 'Node disk space' },
  { kind: 'node_resource_usage', name: 'Node CPU/memory usage' },
]

function readDismissed(): boolean {
  try {
    return window.localStorage.getItem(DISMISSED_STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

function writeDismissed(): void {
  try {
    window.localStorage.setItem(DISMISSED_STORAGE_KEY, '1')
  } catch {
    // A blocked store just means the prompt reappears next session, never
    // a reason to break the dismiss action itself.
  }
}

// Dismissible dashboard nudge for the four platform-wide alert kinds
// (cert_expiry, patch_status, node_disk_space, node_resource_usage),
// none of which are seeded by default and none of which OnboardingFlow
// ever mentions. Shown once at least one app exists rather than as an
// onboarding step: a platform-wide rule still has to be created through
// some app's own POST /api/v1/apps/{name}/alerts URL
// (internal/api/alerts.go's handleCreateAlertRule requires the named app
// to exist), so there is nothing to attach it to before "deploy your
// first app" has already happened. Dismissal persists in
// window.localStorage rather than DockerHealthBanner's own
// React.useState, since an operator who dismisses this should not see it
// again next session either.
export function AlertingQuickSetupPrompt({
  carrierAppName,
}: {
  carrierAppName: string
}) {
  const [dismissed, setDismissed] = useState(readDismissed)
  const channelsQuery = useNotificationChannelsOptional()
  const channels = channelsQuery.data ?? []
  const [channelId, setChannelId] = useState('')
  const createRule = useCreateAlertRule(carrierAppName)
  const [isEnabling, setIsEnabling] = useState(false)

  if (dismissed) {
    return null
  }

  function dismiss() {
    writeDismissed()
    setDismissed(true)
  }

  const selectedChannelId = channelId || channels[0]?.id || ''

  async function handleEnable() {
    if (!selectedChannelId) return
    setIsEnabling(true)
    try {
      for (const rule of RECOMMENDED_RULES) {
        const req: CreateAlertRuleRequest = {
          name: rule.name,
          kind: rule.kind,
          channel_id: selectedChannelId,
          enabled: true,
        }
        await createRule.mutateAsync(req)
      }
      toast.add({ title: 'Recommended alerts enabled.', type: 'success' })
      writeDismissed()
      setDismissed(true)
    } catch (error) {
      toast.add({
        title: 'Could not enable all recommended alerts.',
        description: error instanceof Error ? error.message : undefined,
        type: 'error',
      })
    } finally {
      setIsEnabling(false)
    }
  }

  return (
    <Alert>
      <BellRingingIcon />
      <AlertTitle>Turn on platform-wide alerting</AlertTitle>
      <AlertDescription>
        Certificate expiry, node patch status, node disk space, and node
        CPU/memory usage have no alert rule watching them yet.{' '}
        {channels.length === 0
          ? 'Connect a notification channel first, then come back here to enable all four with one click.'
          : `Enable all four with sensible defaults, notified through ${
              channels.length > 1 ? 'the channel below' : channels[0]?.name
            }.`}
      </AlertDescription>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        {channels.length === 0 ? (
          <CreateNotificationChannelDialog />
        ) : (
          <>
            {channels.length > 1 ? (
              <Select
                value={selectedChannelId}
                onValueChange={(value) => setChannelId(value ?? '')}
              >
                <SelectTrigger className="h-8 w-56 text-xs">
                  <SelectValue placeholder="Choose a channel" />
                </SelectTrigger>
                <SelectContent>
                  {channels.map((channel) => (
                    <SelectItem key={channel.id} value={channel.id}>
                      {channel.name} ({CHANNEL_KIND_LABEL[channel.kind]})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null}
            <Button
              type="button"
              size="sm"
              disabled={isEnabling || !selectedChannelId}
              onClick={() => {
                void handleEnable()
              }}
            >
              {isEnabling ? 'Enabling...' : 'Enable recommended alerts'}
            </Button>
          </>
        )}
        <Link
          to="/apps/$name/alerts"
          params={{ name: carrierAppName }}
          className="text-xs text-muted-foreground hover:text-foreground"
        >
          Or configure manually
        </Link>
      </div>
      <AlertAction>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Dismiss for now"
          title="Dismiss for now"
          onClick={dismiss}
        >
          <XIcon />
        </Button>
      </AlertAction>
    </Alert>
  )
}
