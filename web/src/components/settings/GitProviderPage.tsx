import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { BrandTile } from '../BrandLogoBadge'
import { GitAccessValidate } from '../GitAccessValidate'
import { GitProviderRail } from './GitProviderRail'

export type GitProvider =
  'github' | 'gitlab' | 'gitea' | 'forgejo' | 'bitbucket' | 'bitbucket_server'

// Bitbucket Server has no mark of its own in thesvg, so it shows Bitbucket's.
const BRAND_MARK: Record<GitProvider, string> = {
  github: 'github',
  gitlab: 'gitlab',
  gitea: 'gitea',
  forgejo: 'forgejo',
  bitbucket: 'bitbucket',
  bitbucket_server: 'bitbucket',
}

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
  const copy: Copy = {
    title: t(`gitProviders.${provider}.title`),
    description: t(`gitProviders.${provider}.description`),
    banner: t(`gitProviders.${provider}.banner`),
  }
  return (
    <PageTemplate
      logo={
        <BrandTile name={BRAND_MARK[provider]} label={copy.title} size="lg" />
      }
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
