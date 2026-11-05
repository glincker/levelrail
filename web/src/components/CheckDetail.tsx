import { useId, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import { HelpLink } from '@/components/HelpLink'
import type { DoctorCheck } from '../queries/systemDoctor'
import { systemDoctorKeys } from '../queries/systemDoctor'
import { isShellCommand } from '../lib/shellCommand'
import { getCheckCta } from './doctorCheckCtas'
import { FixAction, ManualFix } from './fixit/FixAction'
import { useFixForCheck } from '../queries/fixit'
import { useDoctorExplanation } from '../queries/doctorExplain'
import { ApiErrorAlert } from './ApiErrorAlert'
import { FieldLabel } from './ui/typography'

// Shared by DoctorCheckRow's full-detail rendering (system status page) and
// the setup wizard's collapsed rows, so the CTA copy and fix controls only
// have one place to render, not two drifting copies.

/** CheckDetail renders a check's Explain control plus, when failing, its next step: a typed fix when one exists, else a Re-check button with any command collapsed. */
export function CheckDetail({
  check,
  onRecheck,
}: {
  check: DoctorCheck
  /** Page-level re-check for this row; without it the doctor query is refetched. */
  onRecheck?: () => Promise<void> | void
}) {
  return (
    <>
      <NextStep check={check} onRecheck={onRecheck} />
      <ExplainCheck code={check.code} />
    </>
  )
}

function ExplainCheck({ code }: { code: string }) {
  const { t } = useTranslation('fixit')
  const [open, setOpen] = useState(false)
  const [asked, setAsked] = useState(false)
  const panelId = useId()
  const { data, isFetching, error } = useDoctorExplanation(code, asked)

  function toggle() {
    setOpen((o) => !o)
    setAsked(true)
  }

  return (
    <div className="mt-2">
      <button
        type="button"
        onClick={toggle}
        aria-expanded={open}
        aria-controls={panelId}
        className="inline-flex items-center gap-1 rounded-sm text-[13px] font-medium text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <ChevronDownGlassIcon
          size="1.2em"
          className={
            open
              ? 'rotate-180 transition-transform motion-reduce:transition-none'
              : 'transition-transform motion-reduce:transition-none'
          }
        />
        {t('explain.toggle')}
      </button>
      <div id={panelId} hidden={!open} className="mt-1.5 space-y-2">
        {open && isFetching && !data ? (
          <p role="status" className="text-xs text-muted-foreground">
            {t('explain.loading')}
          </p>
        ) : null}
        {open && error ? <ApiErrorAlert error={error} /> : null}
        {open && data ? (
          <div className="space-y-2 rounded-md bg-muted/50 p-2.5">
            <p className="text-sm">{data.plain}</p>
            <div>
              <FieldLabel>{t('explain.why')}</FieldLabel>
              <p className="text-xs text-muted-foreground">{data.why}</p>
            </div>
            <div>
              <FieldLabel>{t('explain.next')}</FieldLabel>
              <p className="text-xs text-muted-foreground">{data.next}</p>
            </div>
            {data.fix_id ? (
              <div>
                <FieldLabel>{t('explain.fix')}</FieldLabel>
                <p className="font-mono text-xs text-muted-foreground">
                  {data.fix_id} ({data.kind})
                </p>
              </div>
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  )
}

function NextStep({
  check,
  onRecheck,
}: {
  check: DoctorCheck
  /** Page-level re-check for this row; without it the doctor query is refetched. */
  onRecheck?: () => Promise<void> | void
}) {
  const { t } = useTranslation('fixit')
  const cta = getCheckCta(check)
  const engineFix = useFixForCheck(check.code)
  const qc = useQueryClient()
  const [pending, setPending] = useState(false)
  if (engineFix) {
    return (
      <FixAction
        fix={engineFix}
        autoApply
        onResult={onRecheck ? () => void onRecheck() : undefined}
      />
    )
  }
  if (!cta && !check.fix) return null
  const fixIsCommand = check.fix ? isShellCommand(check.fix) : false

  function recheck() {
    setPending(true)
    if (onRecheck) {
      void Promise.resolve(onRecheck()).finally(() => {
        setPending(false)
      })
      return
    }
    void qc
      .invalidateQueries({ queryKey: systemDoctorKeys.all })
      .finally(() => {
        setPending(false)
      })
  }

  return (
    <div className="mt-2 space-y-2">
      {cta ? (
        <div className="rounded-md bg-muted/50 p-2.5">
          <p className="text-xs text-muted-foreground">{cta.message}</p>
          <div className="mt-1.5">{cta.action}</div>
        </div>
      ) : null}
      {check.fix ? (
        <ManualFix
          message={fixIsCommand ? undefined : check.fix}
          command={fixIsCommand ? check.fix : undefined}
          pending={pending}
          onRecheck={recheck}
        />
      ) : null}
      {check.docs_path ? (
        <HelpLink
          path={check.docs_path}
          label={t('learnMore')}
          variant="inline"
        />
      ) : null}
    </div>
  )
}
