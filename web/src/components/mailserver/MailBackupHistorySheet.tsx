import { useTranslation } from 'react-i18next'
import { EllipsisGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Progress } from '@/components/ui/progress'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { formatBytes } from '@/lib/modelPreflight'
import type { MailBackupList } from '../../queries/mailBackup'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  data: MailBackupList
  canWrite: boolean
  onRestore: (id: string) => void
}

/** Backup history with space used; restore lives in each row's menu. */
export function MailBackupHistorySheet({
  open,
  onOpenChange,
  data,
  canWrite,
  onRestore,
}: Props) {
  const { t } = useTranslation('mailServer')
  const { backups, cap_bytes, used_bytes, job } = data
  const pct = cap_bytes > 0 ? Math.min(100, (used_bytes / cap_bytes) * 100) : 0
  const busy = job?.state === 'running'
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>{t('backup.historyTitle')}</SheetTitle>
        </SheetHeader>
        <div className="flex flex-col gap-3 overflow-y-auto px-4 pb-4">
          <div className="flex flex-col gap-1 text-xs">
            <Progress value={pct} aria-label={t('backup.usageLabel')} />
            <span className="text-muted-foreground">
              {t('backup.usage', {
                used: formatBytes(used_bytes),
                cap: formatBytes(cap_bytes),
              })}
            </span>
          </div>
          {backups.length === 0 ? (
            <p className="text-xs text-muted-foreground">{t('backup.empty')}</p>
          ) : (
            <ul
              aria-label={t('backup.historyTitle')}
              className="divide-y divide-border rounded-lg border border-border"
            >
              {backups.map((b) => (
                <li key={b.id} className="flex items-center gap-2 px-3 py-2">
                  <div className="min-w-0 flex-1 text-xs">
                    <p className="flex flex-wrap items-center gap-2">
                      <span className="font-mono">{b.id}</span>
                      <span className="text-muted-foreground">
                        {new Date(b.created_at).toLocaleString()}
                      </span>
                      {b.verified ? (
                        <Badge variant="success">{t('backup.verified')}</Badge>
                      ) : null}
                    </p>
                    <p className="truncate text-muted-foreground">
                      {formatBytes(b.stored_bytes)} · mox {b.mox_version} ·{' '}
                      {b.where}
                    </p>
                  </div>
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={
                        <Button
                          size="icon-sm"
                          variant="ghost"
                          aria-label={t('backup.actionsFor', { id: b.id })}
                        />
                      }
                    >
                      <EllipsisGlassIcon size="1.2em" />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        disabled={!canWrite || busy}
                        onClick={() => onRestore(b.id)}
                      >
                        {t('backup.restoreFor', { id: b.id })}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </li>
              ))}
            </ul>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
