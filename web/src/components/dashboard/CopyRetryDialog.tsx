import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { closeCopyRetry, useCopyRetry } from '../../lib/copyRetry'
import { useDatabaseCopies } from '../../queries/migration'
import { CopyForm } from '../MigrationDataCard'

/** CopyRetryDialog retries a failed data copy in place: the source is prefilled, only the password is needed again. */
export function CopyRetryDialog() {
  const { t } = useTranslation('dashboard')
  const database = useCopyRetry()
  const copies = useDatabaseCopies()
  const db = copies.data?.find((c) => c.database === database)
  return (
    <Dialog
      open={database !== null}
      onOpenChange={(open) => {
        if (!open) closeCopyRetry()
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {t('copyRetry.title', { name: database ?? '' })}
          </DialogTitle>
          <DialogDescription>{t('copyRetry.description')}</DialogDescription>
        </DialogHeader>
        {db ? (
          <CopyForm db={db} onDone={closeCopyRetry} />
        ) : (
          <p className="text-sm text-muted-foreground">
            {copies.isPending ? t('copyRetry.loading') : t('copyRetry.missing')}
          </p>
        )}
      </DialogContent>
    </Dialog>
  )
}
