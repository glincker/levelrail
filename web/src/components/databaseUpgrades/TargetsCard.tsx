import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { DatabaseUpgrades } from '../../types/databaseUpgrades'
import { UpgradeNowDialog } from './UpgradeNowDialog'
import { kindVariant } from './upgradeHelpers'

/** TargetsCard lists the versions this database can move to, with a typed-confirm upgrade for patch and minor. */
export function TargetsCard({
  databaseName,
  data,
}: {
  databaseName: string
  data: DatabaseUpgrades
}) {
  const { t } = useTranslation('databaseUpgrades')
  const { targets } = data.advice
  const busy = Boolean(data.active) || data.blockers.length > 0

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('targets.title')}</CardTitle>
        <CardDescription>{t('targets.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {targets.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('targets.empty')}</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('targets.columns.version')}</TableHead>
                <TableHead>{t('targets.columns.kind')}</TableHead>
                <TableHead>{t('targets.columns.details')}</TableHead>
                <TableHead className="text-right">
                  {t('targets.columns.actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {targets.map((target) => (
                <TableRow key={target.version}>
                  <TableCell className="font-mono">{target.version}</TableCell>
                  <TableCell>
                    <Badge variant={kindVariant(target.kind)}>
                      {t(`targets.kind.${target.kind}`)}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap items-center gap-1.5">
                      {target.security ? (
                        <Badge variant="destructive">
                          {t('targets.security')}
                        </Badge>
                      ) : null}
                      {(target.advisories ?? []).map((id) => (
                        <Badge key={id} variant="outline" className="font-mono">
                          {id}
                        </Badge>
                      ))}
                      {target.automatic ? (
                        <Badge variant="muted">{t('targets.automatic')}</Badge>
                      ) : null}
                      {target.eol ? (
                        <span className="text-xs text-muted-foreground">
                          {t('targets.eol', { date: target.eol })}
                        </span>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell className="text-right">
                    {target.kind === 'major' ? (
                      <span className="text-xs text-muted-foreground">
                        {t('targets.majorHint')}
                      </span>
                    ) : (
                      <UpgradeNowDialog
                        databaseName={databaseName}
                        version={target.version}
                        disabled={busy}
                      />
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}
