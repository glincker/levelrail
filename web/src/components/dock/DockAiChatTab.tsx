import { Suspense } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { GearIcon, RobotIcon } from '@phosphor-icons/react/dist/ssr'
import { AiChatPanel } from '../AiChatPanel'
import { useAiAssistantSettings } from '../../queries/aiAssistantSettings'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'

function Body() {
  const { t } = useTranslation()
  const { data: settings } = useAiAssistantSettings()
  if (settings.configured) return <AiChatPanel />
  return (
    <EmptyState
      icon={<RobotIcon className="size-5" />}
      title={t('dock.ai.notConfiguredTitle')}
      description={t('dock.ai.notConfiguredDescription')}
      action={
        <Button size="sm" render={<Link to="/settings/ai-assistant" />}>
          <GearIcon />
          {t('dock.ai.configure')}
        </Button>
      }
      className="flex-1"
    />
  )
}

export function DockAiChatTab() {
  return (
    <div className="flex h-full min-h-0 flex-col p-3">
      <Suspense fallback={<Skeleton className="h-full w-full" />}>
        <Body />
      </Suspense>
    </div>
  )
}
