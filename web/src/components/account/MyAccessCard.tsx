import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  MinusCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { myAccessQueryOptions } from '../../queries/myAccess'

export function MyAccessCard() {
  const { t } = useTranslation('settings')
  const { data, isPending, isError } = useQuery(myAccessQueryOptions())
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('accountPage.access.title')}</CardTitle>
        <CardDescription>{t('accountPage.access.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5 text-sm">
        {isPending ? (
          <p className="text-muted-foreground">{t('accountPage.loading')}</p>
        ) : null}
        {isError ? (
          <p className="text-destructive">{t('accountPage.loadError')}</p>
        ) : null}
        {data ? (
          <>
            <p>
              {t('accountPage.access.tier')}{' '}
              <Badge>
                {t(`accountPage.access.tiers.${data.tier}`, {
                  defaultValue: data.tier,
                })}
              </Badge>
            </p>
            <div className="space-y-1">
              <h3 className="font-medium">{t('accountPage.access.teams')}</h3>
              {data.teams.length === 0 ? (
                <p className="text-muted-foreground">
                  {t('accountPage.access.noTeams')}
                </p>
              ) : (
                <ul>
                  {data.teams.map((team) => (
                    <li key={team.id}>
                      {team.name}{' '}
                      <span className="text-muted-foreground">
                        ({team.role})
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            <div className="space-y-1">
              <h3 className="font-medium">
                {t('accountPage.access.policies')}
              </h3>
              {data.policies.length === 0 ? (
                <p className="text-muted-foreground">
                  {t('accountPage.access.noPolicies')}
                </p>
              ) : (
                <ul>
                  {data.policies.map((p) => (
                    <li key={p.id}>{p.name}</li>
                  ))}
                </ul>
              )}
            </div>
            <ul className="divide-y divide-border">
              {data.capabilities.map((c) => (
                <li key={c.key} className="flex items-start gap-2 py-2">
                  {c.allowed ? (
                    <CheckCircleIcon
                      className="mt-0.5 size-4 shrink-0 text-emerald-600"
                      aria-hidden="true"
                    />
                  ) : (
                    <MinusCircleIcon
                      className="mt-0.5 size-4 shrink-0 text-muted-foreground"
                      aria-hidden="true"
                    />
                  )}
                  <div>
                    <p className="font-medium">
                      {t(`accountPage.access.areas.${c.key}`, {
                        defaultValue: c.key,
                      })}
                    </p>
                    {c.allowed ? null : (
                      <p className="text-xs text-muted-foreground">
                        {t('accountPage.access.missing', {
                          ability: c.required_ability,
                        })}
                      </p>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
