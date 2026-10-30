import { Suspense } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { BotGlassIcon, SettingsGlassIcon } from '@thesvg/ui-react'
import { AiChatPanel } from '../AiChatPanel'
import { noteDockReply } from './dockStore'
import { useAiAssistantSettings } from '../../queries/aiAssistantSettings'
import { useExperimentalFeatures } from '../../hooks/useExperimental'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'

function Body() {
  const { t } = useTranslation()
  const { data: settings } = useAiAssistantSettings()
  if (settings.configured)
    return <AiChatPanel onAssistantReply={noteDockReply} />
  return (
    <EmptyState
      icon={<BotGlassIcon className="size-6" />}
      title={t('dock.ai.notConfiguredTitle')}
      description={t('dock.ai.notConfiguredDescription')}
      action={
        <Button size="sm" render={<Link to="/settings/ai-assistant" />}>
          <SettingsGlassIcon size="1.4em" />
          {t('dock.ai.configure')}
        </Button>
      }
      className="flex-1"
    />
  )
}

export function DockAiChatTab() {
  const { t } = useTranslation()
  const experimental = useExperimentalFeatures()
  if (!experimental.includes('ai-chat')) {
    return (
      <div className="flex h-full min-h-0 flex-col p-3">
        <EmptyState
          icon={<BotGlassIcon className="size-6" />}
          title={t('dock.ai.experimentalOffTitle')}
          description={t('dock.ai.experimentalOffDescription')}
          action={
            <Button size="sm" render={<Link to="/settings/beta-features" />}>
              <SettingsGlassIcon size="1.4em" />
              {t('dock.ai.enableInSettings')}
            </Button>
          }
          className="flex-1"
        />
      </div>
    )
  }
  return (
    <div className="flex h-full min-h-0 flex-col p-3">
      <Suspense fallback={<Skeleton className="h-full w-full" />}>
        <Body />
      </Suspense>
    </div>
  )
}
