import { type FormEvent, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { FileArchiveGlassIcon, ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { useIsRoot } from '../../hooks/useIsRoot'
import {
  accessReviewsQueryOptions,
  packDownloadUrl,
  useCreatePack,
  useEvidencePacks,
  useStartAccessReview,
  useVerifyPack,
} from '../../queries/compliance'
import { formatDate } from '../../lib/format'
import { AccessReviewDetailView } from './AccessReviewDetail'

function ReviewsCard() {
  const { t } = useTranslation('compliance')
  const { data } = useQuery(accessReviewsQueryOptions())
  const start = useStartAccessReview()
  const [name, setName] = useState('')
  const [dueDays, setDueDays] = useState('14')
  const [repeatDays, setRepeatDays] = useState('0')
  const [openId, setOpenId] = useState<string | null>(null)
  const onStart = (e: FormEvent) => {
    e.preventDefault()
    start.mutate(
      {
        name,
        due_days: Number.parseInt(dueDays, 10) || 14,
        repeat_days: Number.parseInt(repeatDays, 10) || 0,
      },
      {
        onSuccess: (r) => {
          setOpenId(r.id)
          setName('')
        },
        onError: (err) => toast.add({ title: err.message, type: 'error' }),
      },
    )
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('review.title')}</CardTitle>
        <CardDescription>{t('review.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <form className="flex flex-wrap items-end gap-3" onSubmit={onStart}>
          <Field>
            <FieldLabel htmlFor="review-name">{t('review.name')}</FieldLabel>
            <Input
              id="review-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="review-due">{t('review.dueDays')}</FieldLabel>
            <Input
              id="review-due"
              inputMode="numeric"
              value={dueDays}
              onChange={(e) => setDueDays(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="review-repeat">
              {t('review.repeatDays')}
            </FieldLabel>
            <Input
              id="review-repeat"
              inputMode="numeric"
              value={repeatDays}
              onChange={(e) => setRepeatDays(e.target.value)}
            />
          </Field>
          <Button type="submit" disabled={start.isPending}>
            {t('review.start')}
          </Button>
        </form>
        {(data ?? []).length === 0 ? (
          <EmptyState
            icon={
              <ShieldCheckGlassIcon
                className="text-tone-success"
                size="1.4em"
              />
            }
            title={t('review.title')}
            description={t('review.empty')}
          />
        ) : (
          <ul className="divide-y divide-border rounded-lg border border-border">
            {(data ?? []).map((r) => (
              <li key={r.id} className="space-y-3 px-3 py-2">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{r.name}</p>
                    <p className="text-xs text-muted-foreground">
                      {t('review.due')}: {formatDate(r.due_at, '')} ·{' '}
                      {r.decided}/{r.total}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    {r.overdue ? (
                      <Badge variant="warning">{t('review.overdue')}</Badge>
                    ) : null}
                    <Badge
                      variant={r.status === 'completed' ? 'success' : 'muted'}
                    >
                      {t(`review.status.${r.status}`)}
                    </Badge>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => setOpenId(openId === r.id ? null : r.id)}
                    >
                      {openId === r.id ? t('review.hide') : t('review.open')}
                    </Button>
                  </div>
                </div>
                {openId === r.id ? <AccessReviewDetailView id={r.id} /> : null}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

function isoDay(offsetDays: number): string {
  const d = new Date()
  d.setDate(d.getDate() + offsetDays)
  return d.toISOString().slice(0, 10)
}

function PacksCard() {
  const { t } = useTranslation('compliance')
  const { data } = useEvidencePacks()
  const create = useCreatePack()
  const verify = useVerifyPack()
  const [from, setFrom] = useState(isoDay(-90))
  const [to, setTo] = useState(isoDay(0))
  const onCreate = (e: FormEvent) => {
    e.preventDefault()
    create.mutate(
      { from, to },
      { onError: (err) => toast.add({ title: err.message, type: 'error' }) },
    )
  }
  const onVerify = (id: string) =>
    verify.mutate(id, {
      onSuccess: (r) =>
        toast.add({
          title: r.valid ? t('packs.verifyOk') : t('packs.verifyFailed'),
          type: r.valid ? 'success' : 'error',
        }),
      onError: (err) => toast.add({ title: err.message, type: 'error' }),
    })
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('packs.title')}</CardTitle>
        <CardDescription>{t('packs.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <Alert>
          <AlertDescription>{t('packs.notCertification')}</AlertDescription>
        </Alert>
        <form className="flex flex-wrap items-end gap-3" onSubmit={onCreate}>
          <Field>
            <FieldLabel htmlFor="pack-from">{t('packs.from')}</FieldLabel>
            <Input
              id="pack-from"
              type="date"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="pack-to">{t('packs.to')}</FieldLabel>
            <Input
              id="pack-to"
              type="date"
              value={to}
              onChange={(e) => setTo(e.target.value)}
            />
          </Field>
          <Button type="submit" disabled={create.isPending}>
            {t('packs.generate')}
          </Button>
        </form>
        {(data ?? []).length === 0 ? (
          <EmptyState
            icon={<FileArchiveGlassIcon size="1.4em" />}
            title={t('packs.title')}
            description={t('packs.empty')}
          />
        ) : (
          <ul className="divide-y divide-border rounded-lg border border-border">
            {(data ?? []).map((p) => (
              <li
                key={p.id}
                className="flex flex-wrap items-center justify-between gap-3 px-3 py-2"
              >
                <div className="min-w-0">
                  <p className="truncate font-mono text-sm">{p.id}</p>
                  <p className="text-xs text-muted-foreground">
                    {p.range_from.slice(0, 10)} {t('packs.rangeTo')}{' '}
                    {p.range_to.slice(0, 10)} ·{' '}
                    {p.status === 'running'
                      ? `${p.step} ${p.progress}%`
                      : formatDate(p.finished_at, '')}
                  </p>
                  {p.error ? (
                    <p role="alert" className="text-xs text-destructive">
                      {p.error}
                    </p>
                  ) : null}
                </div>
                <div className="flex items-center gap-2">
                  <Badge
                    variant={
                      p.status === 'complete'
                        ? 'success'
                        : p.status === 'failed'
                          ? 'destructive'
                          : 'muted'
                    }
                  >
                    {t(`packs.status.${p.status}`)}
                  </Badge>
                  {p.status === 'complete' ? (
                    <>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={verify.isPending}
                        onClick={() => onVerify(p.id)}
                      >
                        {t('packs.verify')}
                      </Button>
                      <a
                        href={packDownloadUrl(p.id)}
                        download
                        className={buttonVariants({ size: 'sm' })}
                      >
                        {t('packs.download')}
                      </a>
                    </>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

export function CompliancePanel() {
  const { t } = useTranslation('compliance')
  const isRoot = useIsRoot()
  if (!isRoot) {
    return (
      <Alert>
        <AlertDescription>{t('rootOnly')}</AlertDescription>
      </Alert>
    )
  }
  return (
    <div className="space-y-6">
      <ReviewsCard />
      <PacksCard />
    </div>
  )
}
