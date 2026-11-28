import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useServiceTemplate } from '../../queries/serviceTemplates'
import { TemplateLogoTile } from '../TemplateLogoTile'

const TIER_VARIANT = {
  review: 'warning',
  elevated: 'destructive',
  privileged: 'destructive',
} as const

/** Shows a linked catalog template with its platforms and grants, then hands off to its install page. */
export default function DeployTemplateCard({ id }: { id: string }) {
  const { t } = useTranslation('newApp')
  const { data, isPending, isError } = useServiceTemplate(id)
  if (isPending) {
    return (
      <div role="status" aria-label={t('deployTemplate.loading')}>
        <Skeleton className="h-24 w-full rounded-xl" />
      </div>
    )
  }
  if (isError || !data) {
    return (
      <p role="alert" className="text-sm text-muted-foreground">
        {t('deployTemplate.notFound', { id })}
      </p>
    )
  }
  const platforms = Object.entries(data.arch ?? {}).filter(
    ([, v]) => v !== 'absent',
  )
  const grants = data.requirements.capabilities ?? []
  return (
    <section
      aria-label={t('deployTemplate.title')}
      className="space-y-3 rounded-container border border-border bg-card p-4 shadow-card"
    >
      <div className="flex items-center gap-3">
        <TemplateLogoTile id={data.id} name={data.name} size="sm" />
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-sm font-semibold">{data.name}</h2>
          <p className="truncate text-xs text-muted-foreground">
            {data.slogan}
          </p>
        </div>
        <Link
          to="/templates/$id"
          params={{ id: data.id }}
          className={buttonVariants({ size: 'sm' })}
        >
          {t('deployTemplate.install')}
        </Link>
      </div>
      <div className="flex flex-wrap items-center gap-1.5 text-xs">
        <span className="text-muted-foreground">
          {t('deployTemplate.platforms')}
        </span>
        <ul className="flex flex-wrap gap-1.5">
          {platforms.map(([arch]) => (
            <li key={arch}>
              <Badge variant="muted">{arch}</Badge>
            </li>
          ))}
        </ul>
        <span className="ms-2 text-muted-foreground">
          {t('deployTemplate.grants')}
        </span>
        {grants.length === 0 ? (
          <span>{t('deployTemplate.noGrants')}</span>
        ) : (
          <ul className="flex flex-wrap gap-1.5">
            {grants.map((g) => (
              <li key={`${g.service}-${g.class}`}>
                <Badge variant={TIER_VARIANT[g.tier]} title={g.reason}>
                  {g.class}
                </Badge>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  )
}
