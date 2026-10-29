import { useContext, useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { SocialGithubGlassIcon } from '@thesvg/ui-react'
import { BrandGlow } from '@/components/ui/brand-glow'
import { buttonVariants } from '@/components/ui/button'
import { BrandContext } from '@/lib/brandContext'
import { sanitizeSvg } from '@/lib/sanitizeSvg'
import { cn } from '@/lib/utils'

export interface UpdateBrandBannerProps {
  version: string
  statePill: ReactNode
  stateIcon: ReactNode
  /** Tone classes for the state badge on the logo tile. */
  stateClassName: string
}

/** UpdateBrandBanner is the branded top strip of the Updates card: logo, running version, state and a repository link. */
export function UpdateBrandBanner({
  version,
  statePill,
  stateIcon,
  stateClassName,
}: UpdateBrandBannerProps) {
  const { t } = useTranslation('updates')
  const brand = useContext(BrandContext)
  const logoSvg = useMemo(() => sanitizeSvg(brand?.LogoSVG ?? ''), [brand])
  const initial = (brand?.ShortName || brand?.Name || 'L').charAt(0)
  const repo = brand?.RepoURL

  return (
    <div
      data-testid="update-brand-banner"
      className="relative flex flex-wrap items-center gap-x-4 gap-y-3 overflow-hidden rounded-xl bg-black p-4 text-[#cfcfd3] ring-1 ring-white/10"
    >
      <BrandGlow />
      <div className="relative flex min-w-0 items-center gap-3">
        <span className="relative flex size-12 shrink-0 items-center justify-center rounded-xl bg-white/10 text-lg font-semibold text-white ring-1 ring-white/15">
          {logoSvg ? (
            <span
              className="flex size-7 items-center justify-center [&_svg]:size-full"
              dangerouslySetInnerHTML={{ __html: logoSvg }}
            />
          ) : (
            initial
          )}
          <span
            data-testid="update-state-icon"
            className={cn(
              'absolute -right-1.5 -bottom-1.5 flex size-5 items-center justify-center rounded-full ring-2 ring-black [&_svg]:size-3.5',
              stateClassName,
            )}
          >
            {stateIcon}
          </span>
        </span>
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span title={t('status.runningVersion')}>
            <span className="sr-only">{t('status.runningVersion')}</span>
            <code className="rounded-md bg-white/10 px-2 py-1 font-mono text-sm text-white ring-1 ring-white/15">
              {version}
            </code>
          </span>
          {statePill}
        </div>
      </div>
      {repo ? (
        <a
          href={repo}
          target="_blank"
          rel="noopener noreferrer"
          aria-label={t('status.githubRepo')}
          title={t('status.githubRepo')}
          className={cn(
            buttonVariants({ variant: 'outline', size: 'icon-sm' }),
            'relative ml-auto border-white/15 bg-white/5 text-white hover:bg-white/10 hover:text-white',
          )}
        >
          <SocialGithubGlassIcon size="1.2em" />
        </a>
      ) : null}
    </div>
  )
}
