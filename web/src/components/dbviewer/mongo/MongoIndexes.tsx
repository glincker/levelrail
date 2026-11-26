import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatIndexKey } from '../../../lib/mongoExplorer'
import type { MongoCollectionInfo } from '../../../types/databaseMongo'

export function MongoIndexes({ info }: { info: MongoCollectionInfo }) {
  const { t } = useTranslation('databases')
  if (info.indexes.length === 0) {
    return (
      <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
        {t('viewer.mongo.indexes.empty')}
      </p>
    )
  }
  return (
    <Table aria-label={t('viewer.mongo.indexes.label')}>
      <TableHeader>
        <TableRow>
          <TableHead>{t('viewer.mongo.indexes.name')}</TableHead>
          <TableHead>{t('viewer.mongo.indexes.keys')}</TableHead>
          <TableHead>{t('viewer.mongo.indexes.unique')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {info.indexes.map((ix) => (
          <TableRow key={ix.name}>
            <TableCell className="font-medium">{ix.name}</TableCell>
            <TableCell className="font-mono text-xs">
              {formatIndexKey(ix.key)}
            </TableCell>
            <TableCell>
              {ix.unique ? (
                <Badge variant="info">{t('viewer.mongo.indexes.yes')}</Badge>
              ) : (
                <span className="text-muted-foreground">
                  {t('viewer.mongo.indexes.no')}
                </span>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
