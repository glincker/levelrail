import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import { myAccessQueryOptions } from '../../queries/myAccess'
import { publicOAuthProvidersQueryOptions } from '../../queries/oauth'
import { CardError, CardLoading } from './AccountCardStates'

export function LinkedProvidersCard() {
  const { t } = useTranslation('settings')
  const accessQ = useQuery(myAccessQueryOptions())
  const providersQ = useQuery(publicOAuthProvidersQueryOptions())
  const access = accessQ.data
  const providers = providersQ.data
  const linked = new Set(access?.linked_providers ?? [])
  const enabled = (providers ?? []).filter((p) => p.enabled)
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('accountPage.providers.title')}</CardTitle>
        <CardDescription>
          {t('accountPage.providers.description')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {accessQ.isPending || providersQ.isPending ? <CardLoading /> : null}
        {accessQ.isError || providersQ.isError ? (
          <CardError
            retrying={accessQ.isFetching || providersQ.isFetching}
            onRetry={() => {
              void accessQ.refetch()
              void providersQ.refetch()
            }}
          />
        ) : null}
        {access && providers && enabled.length === 0 ? (
          <div className="flex flex-wrap items-center gap-3">
            <p className="text-sm text-muted-foreground">
              {t('accountPage.providers.none')}
            </p>
            <Link
              to="/settings/oauth"
              className={buttonVariants({ variant: 'outline', size: 'sm' })}
            >
              {t('accountPage.providers.configure')}
            </Link>
          </div>
        ) : null}
        {access && providers && enabled.length > 0 ? (
          <ul className="divide-y divide-border text-sm">
            {enabled.map((p) => (
              <li key={p.provider} className="flex items-center gap-3 py-2">
                <span className="flex-1 font-medium">
                  {p.display_name || p.provider}
                </span>
                {linked.has(p.provider) ? (
                  <Badge variant="success">
                    {t('accountPage.providers.linked')}
                  </Badge>
                ) : (
                  <a
                    className={buttonVariants({
                      variant: 'outline',
                      size: 'sm',
                    })}
                    href={`/api/v1/auth/oauth/${encodeURIComponent(p.provider)}/link/start`}
                  >
                    {t('accountPage.providers.link')}
                  </a>
                )}
              </li>
            ))}
          </ul>
        ) : null}
      </CardContent>
    </Card>
  )
}
