import { requireExperimental } from '../../lib/experimental'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  BotGlassIcon,
  CircleCheckGlassIcon,
  CircleMinusGlassIcon,
  LockGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { AiAssistantSettings } from '../../components/ai/AiAssistantSettings'
import { deriveHeader } from '../../components/ai/aiProviderState'
import { BrandTile } from '../../components/BrandLogoBadge'
import { aiProviderBrand } from '../../lib/providerBrands'
import { useAiConfig } from '../../queries/aiAssistantSettings'
import { useAiProviders } from '../../queries/aiProviders'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { StatusBadge } from '@/components/ui/status-badge'

export const Route = createFileRoute('/settings/ai-assistant')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'ai-chat', location.href),
  component: AiAssistantSettingsPage,
})

function HeaderStatus() {
  const { t } = useTranslation('settings')
  const providers = useAiProviders()
  const config = useAiConfig()
  if (providers.isPending) return null
  const list = providers.data ?? []
  const state = deriveHeader(list, config.data)
  if (state.kind === 'ready') {
    const owner = list.find((p) => p.id === state.active.provider)
    return (
      <span className="inline-flex items-center gap-2">
        <StatusBadge
          tone="success"
          icon={CircleCheckGlassIcon}
          label={t('aiAssistantPage.header.ready')}
        />
        <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
          <BrandTile
            name={aiProviderBrand(owner?.brand)}
            label={owner?.label ?? state.active.provider}
            size="xs"
          />
          {state.active.model}
        </span>
      </span>
    )
  }
  if (state.kind === 'attention') {
    return (
      <StatusBadge
        tone="warning"
        icon={TriangleAlertGlassIcon}
        label={t('aiAssistantPage.header.attention')}
      />
    )
  }
  return (
    <StatusBadge
      tone="neutral"
      icon={CircleMinusGlassIcon}
      label={t('aiAssistantPage.notConfigured')}
    />
  )
}

function AiAssistantSettingsPage() {
  const { t } = useTranslation('settings')
  return (
    <PageTemplate
      icon={<BotGlassIcon size="1.2em" />}
      title={t('aiAssistantPage.title')}
      description={
        <>
          {t('aiAssistantPage.description')}
          <span className="mt-1 flex items-center gap-1.5 text-xs">
            <LockGlassIcon className="size-4" aria-hidden="true" />
            {t('aiAssistantPage.reassurance')}
          </span>
        </>
      }
      status={<HeaderStatus />}
    >
      <AiAssistantSettings />
    </PageTemplate>
  )
}
