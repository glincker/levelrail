import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ShieldAlertGlassIcon, ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import {
  fetchAuditChainReport,
  type AuditChainReport,
} from '@/queries/auditLog'

function toSeq(raw: string): number | undefined {
  const n = Number.parseInt(raw, 10)
  return Number.isFinite(n) && n > 0 ? n : undefined
}

// Runs the hash-chain check on demand, optionally limited to a sequence range.
export function AuditVerifySheet() {
  const { t } = useTranslation('auditLog')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [report, setReport] = useState<AuditChainReport | null>(null)

  async function run() {
    setPending(true)
    setError(null)
    try {
      setReport(
        await fetchAuditChainReport({ from: toSeq(from), to: toSeq(to) }),
      )
    } catch (err) {
      setReport(null)
      setError(err instanceof Error ? err.message : t('verify.failedToRun'))
    } finally {
      setPending(false)
    }
  }

  return (
    <Sheet>
      <SheetTrigger
        render={<Button type="button" size="sm" variant="outline" />}
      >
        <ShieldCheckGlassIcon size="1.4em" aria-hidden="true" />
        {t('verify.button')}
      </SheetTrigger>
      <SheetContent className="gap-4">
        <SheetHeader>
          <SheetTitle>{t('verify.title')}</SheetTitle>
          <SheetDescription>{t('verify.description')}</SheetDescription>
        </SheetHeader>
        <div className="flex flex-wrap items-end gap-2 px-4">
          <label className="flex flex-col gap-1 text-xs">
            {t('verify.from')}
            <Input
              inputMode="numeric"
              value={from}
              onChange={(e) => {
                setFrom(e.target.value)
              }}
              className="w-28"
            />
          </label>
          <label className="flex flex-col gap-1 text-xs">
            {t('verify.to')}
            <Input
              inputMode="numeric"
              value={to}
              onChange={(e) => {
                setTo(e.target.value)
              }}
              className="w-28"
            />
          </label>
          <Button
            type="button"
            size="sm"
            disabled={pending}
            onClick={() => {
              void run()
            }}
          >
            {pending ? t('verify.running') : t('verify.run')}
          </Button>
        </div>
        <div className="space-y-2 px-4 text-sm" aria-live="polite">
          {error ? <p className="text-destructive">{error}</p> : null}
          {report?.ok ? (
            <>
              <p className="flex items-center gap-1.5 font-medium">
                <ShieldCheckGlassIcon
                  className="text-tone-success"
                  size="1.4em"
                  aria-hidden="true"
                />
                {t('verify.okSummary', { count: report.checked })}
              </p>
              {report.range_end ? (
                <p className="text-muted-foreground">
                  {t('verify.rangeEnd', { seq: report.range_end })}
                </p>
              ) : null}
              {report.range_end_hash ? (
                <code className="block rounded bg-muted p-2 text-xs break-all">
                  {report.range_end_hash}
                </code>
              ) : null}
              <p className="text-muted-foreground">
                {t('verify.head', { seq: report.head })}
              </p>
              {report.head_hash ? (
                <code className="block rounded bg-muted p-2 text-xs break-all">
                  {report.head_hash}
                </code>
              ) : null}
            </>
          ) : null}
          {report?.ok && (report.problems_outside_range ?? 0) > 0 ? (
            <p
              role="alert"
              className="flex items-center gap-1.5 font-medium text-destructive"
            >
              <ShieldAlertGlassIcon
                className="text-tone-warning"
                size="1.4em"
                aria-hidden="true"
              />
              {t('verify.outside', { count: report.problems_outside_range })}
            </p>
          ) : null}
          {report && !report.ok ? (
            <>
              <p className="flex items-center gap-1.5 font-medium text-destructive">
                <ShieldAlertGlassIcon
                  className="text-tone-warning"
                  size="1.4em"
                  aria-hidden="true"
                />
                {report.first_break
                  ? t('verify.firstBreak', { seq: report.first_break.seq })
                  : t('verify.failed')}
              </p>
              <ul className="space-y-1 text-xs">
                {report.problems.map((p) => (
                  <li key={`${p.seq}-${p.reason}`}>
                    {t('verify.problem', { seq: p.seq, reason: p.reason })}
                  </li>
                ))}
              </ul>
            </>
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}
