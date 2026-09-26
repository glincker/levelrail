import {
  CheckCircleIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { PlusIcon } from '@phosphor-icons/react/dist/ssr'
import { SkeletonLine, StatusPill } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { useAttentionItems } from '../../queries/attention'
import { CreateResourceWizard } from '../CreateResourceWizard'
import { SetupChip } from './SetupChip'
import { greetingFor, platformPill } from '../../lib/fleetStatus'

export function StatusHeader({
  firstAppName,
  showSetup,
}: {
  firstAppName: string | undefined
  showSetup: boolean
}) {
  const username = useAuthUsername()
  const { items, isLoading } = useAttentionItems()
  const pill = platformPill(items)
  return (
    <header className="flex flex-wrap items-center justify-between gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-lg font-semibold text-foreground">
          {greetingFor(new Date().getHours())}
          {username ? `, ${username}` : ''}
        </h1>
        {isLoading ? (
          <SkeletonLine width={120} className="h-6 rounded-full" />
        ) : (
          <StatusPill
            tone={pill.tone}
            label={pill.label}
            live
            icon={
              pill.tone === 'success' ? (
                <CheckCircleIcon weight="fill" className="size-4" />
              ) : (
                <WarningCircleIcon weight="fill" className="size-4" />
              )
            }
          />
        )}
        {showSetup ? <SetupChip firstAppName={firstAppName} /> : null}
      </div>
      <CreateResourceWizard
        trigger={
          <Button size="sm">
            <PlusIcon />
            New app
          </Button>
        }
      />
    </header>
  )
}
