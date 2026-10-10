import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/toast'
import type { DatabaseCredential, TempPreset } from '../../types/databaseAccess'
import {
  useDatabaseTemp,
  useIssueTemp,
  useRevokeTemp,
} from '../../queries/databaseAccess'

const PRESETS: TempPreset[] = ['read_only', 'read_write']
const MINUTES_PER_HOUR = 60
const TTL_CHOICES = [15, 60, 240, 720, 1440]
const MS_PER_MINUTE = 60_000

function ttlLabel(
  t: (k: 'temp.minutes' | 'temp.hours', o: { count: number }) => string,
  minutes: number,
) {
  return minutes >= MINUTES_PER_HOUR
    ? t('temp.hours', { count: minutes / MINUTES_PER_HOUR })
    : t('temp.minutes', { count: minutes })
}

function timeLeft(expiresAt: string, expiredLabel: string): string {
  const ms = new Date(expiresAt).getTime() - Date.now()
  if (ms <= 0) return expiredLabel
  const minutes = Math.ceil(ms / MS_PER_MINUTE)
  return minutes >= MINUTES_PER_HOUR
    ? `${Math.floor(minutes / MINUTES_PER_HOUR)}h ${minutes % MINUTES_PER_HOUR}m`
    : `${minutes}m`
}

/** TempAccessCard issues short-lived logins and lists the ones still alive. */
export function TempAccessCard({
  databaseName,
  onCredential,
}: {
  databaseName: string
  onCredential: (c: DatabaseCredential) => void
}) {
  const { t } = useTranslation('databaseAccess')
  const { data } = useDatabaseTemp(databaseName)
  const issue = useIssueTemp(databaseName)
  const revoke = useRevokeTemp(databaseName)
  const [preset, setPreset] = useState<TempPreset>('read_only')
  const [minutes, setMinutes] = useState<number | null>(null)

  const limits = data?.limits
  const choices = TTL_CHOICES.filter(
    (m) => !limits || (m >= limits.min_minutes && m <= limits.max_minutes),
  )
  const chosen = minutes ?? limits?.default_minutes ?? 60

  function onIssue() {
    issue.mutate(
      { preset, ttl_minutes: chosen },
      {
        onSuccess: (res) => {
          toast.add({
            title: t('temp.issuedToast'),
            description: res.clamped
              ? t('temp.clampedNote', {
                  min: res.limits.min_minutes,
                  max: res.limits.max_minutes,
                })
              : undefined,
            type: 'success',
          })
          onCredential(res.credential)
        },
        onError: (err) =>
          toast.add({
            title: t('temp.failedToast'),
            description: err.message,
            type: 'error',
          }),
      },
    )
  }

  const items = data?.items ?? []

  return (
    <Card>
      <CardHeader className="space-y-1">
        <CardTitle className="flex items-center gap-1.5 text-sm">
          <KeyIcon className="size-4" aria-hidden="true" />
          {t('temp.title')}
        </CardTitle>
        <p className="text-xs text-muted-foreground">{t('temp.description')}</p>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-end gap-3">
          <div className="space-y-1.5">
            <span className="text-xs font-medium">{t('temp.preset')}</span>
            <Select
              value={preset}
              onValueChange={(v) => setPreset(v as TempPreset)}
            >
              <SelectTrigger className="w-48" aria-label={t('temp.preset')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PRESETS.map((p) => (
                  <SelectItem key={p} value={p}>
                    {t(`users.preset.${p}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <span className="text-xs font-medium">{t('temp.lifetime')}</span>
            <Select
              value={String(chosen)}
              onValueChange={(v) => setMinutes(Number(v))}
            >
              <SelectTrigger className="w-40" aria-label={t('temp.lifetime')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {choices.map((m) => (
                  <SelectItem key={m} value={String(m)}>
                    {ttlLabel(t, m)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <Button disabled={issue.isPending} onClick={onIssue}>
            {issue.isPending ? t('temp.issuing') : t('temp.issue')}
          </Button>
        </div>
        <div className="space-y-2">
          <h3 className="text-xs font-medium">{t('temp.activeTitle')}</h3>
          {items.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              {t('temp.emptyActive')}
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('temp.columns.role')}</TableHead>
                  <TableHead>{t('temp.columns.access')}</TableHead>
                  <TableHead>{t('temp.columns.by')}</TableHead>
                  <TableHead>{t('temp.columns.expires')}</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((row) => (
                  <TableRow key={row.id}>
                    <TableCell className="font-mono text-xs">
                      {row.role}
                    </TableCell>
                    <TableCell className="text-xs">
                      {t(`users.preset.${row.preset}`)}
                    </TableCell>
                    <TableCell className="text-xs">{row.created_by}</TableCell>
                    <TableCell className="text-xs">
                      {timeLeft(row.expires_at, t('temp.expiredWaiting'))}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={revoke.isPending}
                        onClick={() =>
                          revoke.mutate(row.id, {
                            onSuccess: () =>
                              toast.add({
                                title: t('temp.revokedToast'),
                                type: 'success',
                              }),
                          })
                        }
                      >
                        {t('temp.revoke')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
