import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { BrandTile } from '../BrandLogoBadge'
import { GitAccessValidate } from '../GitAccessValidate'
import { GitProviderRail } from './GitProviderRail'

export type GitProvider = 'github' | 'gitlab' | 'gitea' | 'bitbucket'

interface Copy {
  title: string
  description: string
  banner: string
}

// Shared hero for the four git provider settings pages.
export function GitProviderPage({
  provider,
  children,
}: {
  provider: GitProvider
  children: ReactNode
}) {
  const { t } = useTranslation('settings')
  const copy = ((): Copy => {
    switch (provider) {
      case 'github':
        return {
          title: t('gitProviders.github.title'),
          description: t('gitProviders.github.description'),
          banner: t('gitProviders.github.banner'),
        }
      case 'gitlab':
        return {
          title: t('gitProviders.gitlab.title'),
          description: t('gitProviders.gitlab.description'),
          banner: t('gitProviders.gitlab.banner'),
        }
      case 'gitea':
        return {
          title: t('gitProviders.gitea.title'),
          description: t('gitProviders.gitea.description'),
          banner: t('gitProviders.gitea.banner'),
        }
      case 'bitbucket':
        return {
          title: t('gitProviders.bitbucket.title'),
          description: t('gitProviders.bitbucket.description'),
          banner: t('gitProviders.bitbucket.banner'),
        }
    }
  })()
  return (
    <PageTemplate
      logo={<BrandTile name={provider} label={copy.title} size="lg" />}
      eyebrow={t('gitProviders.eyebrow')}
      title={copy.title}
      description={copy.description}
      aside={<GitProviderRail provider={provider} />}
      asideLabel={t('gitProviders.rail.label')}
      banners={
        <Banner
          variant="info"
          dismissId={`settings.git.${provider}.intro`}
          title={copy.banner}
        />
      }
    >
      <div className="space-y-6">
        {children}
        <GitAccessValidate
          path={`/api/v1/git-providers/${provider}/validate`}
        />
      </div>
    </PageTemplate>
  )
}
