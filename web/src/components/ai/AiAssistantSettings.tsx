import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LockGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../../hooks/useIsRoot'
import {
  useAiConfig,
  type AiSelection,
} from '../../queries/aiAssistantSettings'
import { useAiProviders, useUpdateAiRouting } from '../../queries/aiProviders'
import { AiFallbackList } from './AiFallbackList'
import { AiProviderDrawer } from './AiProviderDrawer'
import { AiProviderGrid } from './AiProviderGrid'
import { AiUsagePanel } from './AiUsagePanel'

function Section({
  title,
  hint,
  children,
}: {
  title: string
  hint?: string
  children: React.ReactNode
}) {
  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-sm font-medium">{title}</h2>
        {hint ? <p className="text-sm text-muted-foreground">{hint}</p> : null}
      </div>
      {children}
    </section>
  )
}

export function AiAssistantSettings() {
  const { t } = useTranslation('settings')
  const isRoot = useIsRoot()
  const providersQuery = useAiProviders()
  const configQuery = useAiConfig()
  const routing = useUpdateAiRouting()
  const [openId, setOpenId] = useState<string | null>(null)

  if (providersQuery.isPending) {
    return (
      <div
        className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3"
        aria-hidden="true"
      >
        {Array.from({ length: 6 }, (_, i) => (
          <Skeleton key={i} className="h-28 w-full rounded-xl" />
        ))}
      </div>
    )
  }

  if (providersQuery.isError) {
    return (
      <Alert variant="destructive">
        <TriangleAlertGlassIcon
          className="text-tone-warning"
          size="1.7em"
          aria-hidden="true"
        />
        <AlertDescription className="flex flex-wrap items-center gap-3">
          {t('aiAssistantPage.loadError')}
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              void providersQuery.refetch()
            }}
          >
            {t('aiAssistantPage.retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  const providers = providersQuery.data
  const config = configQuery.data
  const open = providers.find((p) => p.id === openId) ?? null

  function saveFallback(fallback: AiSelection[]) {
    routing.mutate(
      { fallback },
      {
        onError: (e) => {
          toast.add({ title: e.message, type: 'error' })
        },
      },
    )
  }

  return (
    <div className="space-y-8">
      {!isRoot ? (
        <Alert>
          <LockGlassIcon size="1.4em" aria-hidden="true" />
          <AlertDescription>{t('aiAssistantPage.rootOnly')}</AlertDescription>
        </Alert>
      ) : null}

      <AiProviderGrid
        providers={providers}
        activeId={config?.active?.provider}
        onOpen={setOpenId}
      />

      <Section
        title={t('aiAssistantPage.fallback.title')}
        hint={t('aiAssistantPage.fallback.hint')}
      >
        <AiFallbackList
          fallback={config?.fallback ?? []}
          providers={providers}
          canWrite={isRoot}
          pending={routing.isPending}
          onChange={saveFallback}
        />
      </Section>

      <Section title={t('aiAssistantPage.usage.title')}>
        <AiUsagePanel
          budget={config?.limits.monthly_token_budget}
          monthTotal={config?.token_usage_month}
        />
      </Section>

      <AiProviderDrawer
        provider={open}
        onOpenChange={(next) => {
          if (!next) setOpenId(null)
        }}
        canWrite={isRoot}
        config={config}
      />
    </div>
  )
}
