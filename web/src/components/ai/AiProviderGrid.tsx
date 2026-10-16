import { useTranslation } from 'react-i18next'
import type { AiProvider } from '../../queries/aiProviders'
import { AiProviderCard } from './AiProviderCard'
import { groupProviders } from './aiProviderState'

function Group({
  title,
  providers,
  activeId,
  onOpen,
}: {
  title?: string
  providers: AiProvider[]
  activeId: string | undefined
  onOpen: (id: string) => void
}) {
  if (providers.length === 0) return null
  return (
    <section className="space-y-3">
      {title ? (
        <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
          {title}
        </h3>
      ) : null}
      <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {providers.map((p) => (
          <li key={p.id} className="flex">
            <AiProviderCard
              provider={p}
              active={p.id === activeId}
              onOpen={onOpen}
            />
          </li>
        ))}
      </ul>
    </section>
  )
}

export function AiProviderGrid({
  providers,
  activeId,
  onOpen,
}: {
  providers: AiProvider[]
  activeId: string | undefined
  onOpen: (id: string) => void
}) {
  const { t } = useTranslation('settings')
  const { hosted, selfHosted } = groupProviders(providers)
  return (
    <div className="space-y-6">
      <Group
        providers={hosted}
        activeId={activeId}
        onOpen={onOpen}
        title={
          selfHosted.length > 0 ? t('aiAssistantPage.groups.hosted') : undefined
        }
      />
      <Group
        title={t('aiAssistantPage.groups.selfHosted')}
        providers={selfHosted}
        activeId={activeId}
        onOpen={onOpen}
      />
    </div>
  )
}
