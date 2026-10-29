import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CoinsGlassIcon,
  KeyGlassIcon,
  PlugGlassIcon,
  WifiOffGlassIcon,
  ZapGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { BrandTile } from '../BrandLogoBadge'
import { StatusBadge } from '@/components/ui/status-badge'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { aiProviderBrand } from '../../lib/providerBrands'
import type { AiProvider } from '../../queries/aiProviders'
import { pillState, type PillState } from './aiProviderState'

const PILL: Record<
  PillState,
  { tone: 'neutral' | 'success' | 'danger' | 'warning'; icon: UiIcon }
> = {
  not_connected: { tone: 'neutral', icon: PlugGlassIcon },
  connected: { tone: 'success', icon: CircleCheckGlassIcon },
  invalid_key: { tone: 'danger', icon: KeyGlassIcon },
  unreachable: { tone: 'warning', icon: WifiOffGlassIcon },
  quota_exhausted: { tone: 'warning', icon: CoinsGlassIcon },
}

export function ProviderPill({ provider }: { provider: AiProvider }) {
  const { t } = useTranslation('settings')
  const state = pillState(provider)
  const spec = PILL[state]
  return (
    <StatusBadge
      tone={spec.tone}
      icon={spec.icon}
      label={t(`aiAssistantPage.pill.${state}`)}
    />
  )
}

export function AiProviderCard({
  provider,
  active,
  onOpen,
}: {
  provider: AiProvider
  active: boolean
  onOpen: (id: string) => void
}) {
  const { t } = useTranslation('settings')
  const connected = provider.has_key
  return (
    <button
      type="button"
      onClick={() => {
        onOpen(provider.id)
      }}
      aria-label={t('aiAssistantPage.card.open', { name: provider.label })}
      className={cn(
        'group flex w-full flex-col gap-3 rounded-xl border bg-card p-4 text-left transition-[transform,opacity] outline-none hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring motion-reduce:transition-none',
        active ? 'border-tone-success-border' : 'border-border',
      )}
    >
      <span className="flex items-center gap-3">
        <BrandTile
          name={aiProviderBrand(provider.brand)}
          label={provider.label}
          size="sm"
          muted={!connected}
        />
        <span className="min-w-0 flex-1 truncate text-sm font-medium">
          {provider.label}
        </span>
      </span>
      <span className="flex flex-wrap items-center gap-2">
        <ProviderPill provider={provider} />
        {active ? (
          <Badge variant="accent" className="shrink-0">
            <ZapGlassIcon className="size-3.5" aria-hidden="true" />
            {t('aiAssistantPage.card.active')}
          </Badge>
        ) : null}
      </span>
      <span className="text-xs text-muted-foreground">
        {connected
          ? provider.default_model || t('aiAssistantPage.card.manage')
          : t('aiAssistantPage.card.connect')}
      </span>
    </button>
  )
}
