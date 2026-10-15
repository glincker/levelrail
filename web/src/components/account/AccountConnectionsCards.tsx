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
import { tokenListQueryOptions } from '../../queries/tokens'

export function LinkedProvidersCard() {
  const { t } = useTranslation('settings')
  const { data: access } = useQuery(myAccessQueryOptions())
  const { data: providers } = useQuery(publicOAuthProvidersQueryOptions())
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
        {enabled.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('accountPage.providers.none')}
          </p>
        ) : (
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
        )}
      </CardContent>
    </Card>
  )
}

export function MyTokensCard() {
  const { t } = useTranslation('settings')
  const { data: tokens } = useQuery(tokenListQueryOptions())
  const active = (tokens ?? []).filter((tok) => !tok.revoked_at).length
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('accountPage.tokens.title')}</CardTitle>
        <CardDescription>{t('accountPage.tokens.description')}</CardDescription>
      </CardHeader>
      <CardContent className="flex items-center gap-4">
        <p className="text-sm">
          {t('accountPage.tokens.count', { count: active })}
        </p>
        <Link
          to="/settings/tokens"
          className={buttonVariants({ variant: 'outline', size: 'sm' })}
        >
          {t('accountPage.tokens.manage')}
        </Link>
      </CardContent>
    </Card>
  )
}
