import { useTranslation } from 'react-i18next'
import { idleTotalUsd } from '../../lib/cloudAwsOps'
import { useAwsIdle } from '../../queries/cloudAwsOps'
import { StateLine } from './AwsServiceShared'

export function IdleCard({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const idle = useAwsIdle(id)
  const rows = idle.data ?? []
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">
        {t('services.cloudwatch.idleNote')}
      </p>
      <StateLine
        pending={idle.isPending}
        error={idle.error}
        empty={
          rows.length === 0 ? t('services.cloudwatch.idleEmpty') : undefined
        }
      />
      {rows.length > 0 ? (
        <>
          <table className="w-full text-left text-sm">
            <caption className="sr-only">
              {t('services.cloudwatch.idleTitle')}
            </caption>
            <thead className="text-xs text-muted-foreground">
              <tr>
                <th scope="col">{t('services.cloudwatch.idleResource')}</th>
                <th scope="col">{t('services.cloudwatch.idleKind')}</th>
                <th scope="col">{t('services.cloudwatch.idleAction')}</th>
                <th scope="col" className="text-right">
                  {t('services.cloudwatch.idleSaving')}
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={`${r.kind}-${r.id}`}>
                  <td className="font-mono text-xs" title={r.detail}>
                    {r.id}
                  </td>
                  <td className="text-xs">
                    {t(`services.cloudwatch.kind.${r.kind}`)}
                  </td>
                  <td className="text-xs">
                    {t(`services.cloudwatch.action.${r.action}`)}
                  </td>
                  <td className="text-right text-xs">
                    {r.est_monthly_usd.toFixed(2)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="text-sm font-medium">
            {t('services.cloudwatch.idleTotal', {
              usd: idleTotalUsd(rows).toFixed(2),
            })}
          </p>
        </>
      ) : null}
    </div>
  )
}
