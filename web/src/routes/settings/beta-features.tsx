import { useEffect } from 'react'
import { createFileRoute, useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { FlaskConicalGlassIcon } from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { toast } from '@/components/ui/toast'
import { BetaFeatureCard } from '@/components/settings/BetaFeatureCard'
import { ServerProfileCard } from '@/components/settings/ServerProfileCard'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'
import { bannerFor, groupByMaturity } from '../../lib/betaFeatures'
import { safeReturnPath, type BetaFeaturesSearch } from '../../lib/experimental'
import {
  betaFeaturesQueryOptions,
  useBetaFeatures,
  useSetBetaFeature,
} from '../../queries/resourceAdvisor'

export const Route = createFileRoute('/settings/beta-features')({
  validateSearch: (search: Record<string, unknown>): BetaFeaturesSearch => ({
    feature: typeof search.feature === 'string' ? search.feature : undefined,
    from: typeof search.from === 'string' ? search.from : undefined,
  }),
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(betaFeaturesQueryOptions()),
  component: BetaFeaturesPage,
  pendingComponent: () => (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <SettingsCardSkeleton rows={2} />
    </div>
  ),
})

// Longer operator notes (resourceAdvisor namespace) shown instead of the API summary.
type DetailKey =
  | 'settings.aiChatDescription'
  | 'settings.cliConsoleDescription'
  | 'settings.mobileApiDescription'
  | 'settings.samlScimDescription'
  | 'settings.advisorDescription'

const DETAIL_KEYS: Partial<Record<string, DetailKey>> = {
  'ai-chat': 'settings.aiChatDescription',
  'cli-console': 'settings.cliConsoleDescription',
  'mobile-api': 'settings.mobileApiDescription',
  'saml-scim': 'settings.samlScimDescription',
  'resource-advisor': 'settings.advisorDescription',
}

function BetaFeaturesPage() {
  const { t } = useTranslation('betaFeatures')
  const { t: tAdvisor } = useTranslation('resourceAdvisor')
  const router = useRouter()
  const { feature: focus, from } = Route.useSearch()
  const { data: features = [] } = useBetaFeatures()
  const setFeature = useSetBetaFeature()
  const banner = bannerFor(features, focus)
  const back = safeReturnPath(from)

  useEffect(() => {
    if (!focus) return
    document
      .getElementById(`feature-${focus}`)
      ?.scrollIntoView?.({ block: 'center', behavior: 'smooth' })
  }, [focus, features.length])

  function toggle(id: string, name: string, enabled: boolean) {
    setFeature.mutate(
      { feature: id, enabled },
      {
        onSuccess: (res) =>
          toast.add({
            type: 'success',
            title: t(res.enabled ? 'toggle.on' : 'toggle.off', { name }),
          }),
        onError: (err) =>
          toast.add({
            type: 'error',
            title: t('toggle.error', { name, message: err.message }),
          }),
      },
    )
  }

  const takeMeThere = back ? (
    <Button
      type="button"
      size="sm"
      onClick={() => {
        router.history.push(back)
      }}
    >
      {t('banner.takeMeThere')}
    </Button>
  ) : undefined

  return (
    <PageTemplate
      icon={<FlaskConicalGlassIcon size="1.2em" />}
      title={t('title')}
      description={t('description')}
      banners={
        <>
          {banner.kind === 'locked' && back ? (
            <Banner
              variant="warning"
              title={t('banner.title', { name: banner.feature.name })}
            >
              {banner.feature.pending_restart
                ? t('banner.bodyRestart')
                : t('banner.body')}
            </Banner>
          ) : null}
          {banner.kind === 'ready' && back ? (
            <Banner
              variant="success"
              title={t('banner.ready', { name: banner.feature.name })}
              action={takeMeThere}
            />
          ) : null}
          {banner.kind === 'unknown' ? (
            <Banner variant="info" title={t('banner.unknown')} />
          ) : null}
        </>
      }
    >
      <ServerProfileCard />
      {groupByMaturity(features).map((group) => (
        <section
          key={group.maturity}
          aria-labelledby={`beta-group-${group.maturity}`}
          className="space-y-3"
        >
          <h2
            id={`beta-group-${group.maturity}`}
            className="text-sm font-medium text-foreground"
          >
            {t(`groups.${group.maturity}`)}
          </h2>
          {group.items.map((f) => {
            const detailKey = DETAIL_KEYS[f.id]
            return (
              <BetaFeatureCard
                key={f.id}
                feature={f}
                highlighted={f.id === focus}
                pending={setFeature.isPending}
                onToggle={(enabled) => {
                  toggle(f.id, f.name, enabled)
                }}
                detail={detailKey ? tAdvisor(detailKey) : undefined}
              />
            )
          })}
        </section>
      ))}
    </PageTemplate>
  )
}
