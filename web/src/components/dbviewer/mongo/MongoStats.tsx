import { useTranslation } from 'react-i18next'
import { formatSize } from '../../../lib/format'
import type { MongoCollectionInfo } from '../../../types/databaseMongo'

export function MongoStats({ info }: { info: MongoCollectionInfo }) {
  const { t } = useTranslation('databases')
  const rows: [string, string][] = [
    [t('viewer.mongo.stats.documents'), String(info.count)],
    [t('viewer.mongo.stats.dataSize'), formatSize(info.size)],
    [t('viewer.mongo.stats.storageSize'), formatSize(info.storageSize)],
    [t('viewer.mongo.stats.indexes'), String(info.indexes.length)],
    [t('viewer.mongo.stats.type'), info.type],
  ]
  return (
    <div className="space-y-2">
      <dl className="grid max-w-md grid-cols-[auto_1fr] gap-x-6 gap-y-1 text-sm">
        {rows.map(([label, value]) => (
          <div key={label} className="contents">
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="tabular-nums">{value}</dd>
          </div>
        ))}
      </dl>
      {info.error ? (
        <p role="alert" className="font-mono text-xs text-tone-danger">
          {info.error}
        </p>
      ) : null}
    </div>
  )
}
