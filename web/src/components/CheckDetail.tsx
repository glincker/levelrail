import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { HelpLink } from '@/components/HelpLink'
import type { DoctorCheck } from '../queries/systemDoctor'
import { systemDoctorKeys } from '../queries/systemDoctor'
import { isShellCommand } from '../lib/shellCommand'
import { getCheckCta } from './doctorCheckCtas'
import { FixAction, ManualFix } from './fixit/FixAction'
import { useFixForCheck } from '../queries/fixit'

// Shared by DoctorCheckRow's full-detail rendering (system status page) and
// the setup wizard's collapsed rows, so the CTA copy and fix controls only
// have one place to render, not two drifting copies.

/** CheckDetail renders a failing check's next step: a typed fix when one exists, else a Re-check button with any command collapsed. Renders nothing otherwise. */
export function CheckDetail({
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
