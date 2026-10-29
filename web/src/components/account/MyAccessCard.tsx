import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, CircleMinusGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { CardError, CardLoading } from './AccountCardStates'
import { myAccessQueryOptions } from '../../queries/myAccess'

export function MyAccessCard() {
  const { t } = useTranslation('settings')
  const { data, isPending, isError, isFetching, refetch } = useQuery(
    myAccessQueryOptions(),
  )
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('accountPage.access.title')}</CardTitle>
        <CardDescription>{t('accountPage.access.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5 text-sm">
        {isPending ? <CardLoading rows={3} /> : null}
        {isError ? (
          <CardError
            retrying={isFetching}
            onRetry={() => {
              void refetch()
            }}
          />
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
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('accountPage.access.colArea')}</TableHead>
                  <TableHead>{t('accountPage.access.colStatus')}</TableHead>
                  <TableHead className="text-right">
                    <span className="sr-only">
                      {t('accountPage.access.colAction')}
                    </span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.capabilities.map((c) => {
                  const area = t(`accountPage.access.areas.${c.key}`, {
                    defaultValue: c.key,
                  })
                  return (
                    <TableRow key={c.key}>
                      <TableCell className="font-medium">{area}</TableCell>
                      <TableCell>
                        {c.allowed ? (
                          <span className="inline-flex items-center gap-1.5">
                            <CircleCheckGlassIcon
                              className="size-4.5 shrink-0 text-emerald-600"
                              aria-hidden="true"
                            />
                            {t('accountPage.access.allowed')}
                          </span>
                        ) : (
                          <span className="inline-flex items-center gap-1.5 text-muted-foreground">
                            <CircleMinusGlassIcon
                              className="size-4.5 shrink-0"
                              aria-hidden="true"
                            />
                            {t('accountPage.access.missing', {
                              ability: c.required_ability,
                            })}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="text-right">
                        {c.allowed ? null : (
                          <Link
                            to="/settings/roles"
                            aria-label={t('accountPage.access.requestLabel', {
                              area,
                            })}
                            className={buttonVariants({
                              variant: 'outline',
                              size: 'sm',
                            })}
                          >
                            {t('accountPage.access.request')}
                          </Link>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
