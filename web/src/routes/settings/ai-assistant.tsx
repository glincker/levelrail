import { requireExperimental } from '../../lib/experimental'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  LockKeyIcon,
  MinusCircleIcon,
  RobotIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { AiAssistantSettings } from '../../components/ai/AiAssistantSettings'
import {
  deriveHeader,
  normalizeProvider,
} from '../../components/ai/aiProviderState'
import { BrandTile } from '../../components/BrandLogoBadge'
import { aiProviderBrand } from '../../lib/providerBrands'
import { useAiConfig, useAiProviders } from '../../queries/aiProviders'
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
  const list = (providers.data?.providers ?? []).map(normalizeProvider)
  const state = deriveHeader(list, config.data)
  if (state.kind === 'ready') {
    const owner = list.find((p) => p.id === state.active.provider)
    return (
      <span className="inline-flex items-center gap-2">
        <StatusBadge
          tone="success"
          icon={CheckCircleIcon}
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
        icon={WarningIcon}
        label={t('aiAssistantPage.header.attention')}
      />
    )
  }
  return (
    <StatusBadge
      tone="neutral"
      icon={MinusCircleIcon}
      label={t('aiAssistantPage.notConfigured')}
    />
  )
}

function AiAssistantSettingsPage() {
  const { t } = useTranslation('settings')
  return (
    <PageTemplate
      icon={<RobotIcon />}
      title={t('aiAssistantPage.title')}
      description={
        <>
          {t('aiAssistantPage.description')}
          <span className="mt-1 flex items-center gap-1.5 text-xs">
            <LockKeyIcon className="size-3.5" aria-hidden="true" />
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
