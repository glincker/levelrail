import { useState } from 'react'
import { CopyIcon, CheckIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { HelpLink } from '@/components/HelpLink'
import type { DoctorCheck } from '../queries/systemDoctor'
import { getCheckCta } from './doctorCheckCtas'

// Shared by DoctorCheckRow's full-detail rendering (system status page) and
// the setup wizard's collapsed rows, so the CTA copy and fix command only
// have one place to render, not two drifting copies.

function FixCommand({ fix, docsPath }: { fix: string; docsPath?: string }) {
  const [copied, setCopied] = useState(false)

  function copyFix() {
    void navigator.clipboard.writeText(fix).then(() => {
      setCopied(true)
    })
  }

  return (
    <div className="rounded-md bg-muted/50 p-2.5">
      <div className="flex items-start gap-2">
        <code className="min-w-0 flex-1 overflow-x-auto whitespace-pre-wrap break-all text-xs">
          {fix}
        </code>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="shrink-0"
          onClick={copyFix}
        >
          {copied ? <CheckIcon /> : <CopyIcon />}
          {copied ? 'Copied' : 'Copy'}
        </Button>
      </div>
      {docsPath ? (
        <div className="mt-1.5">
          <HelpLink path={docsPath} label="Learn more" variant="inline" />
        </div>
      ) : null}
    </div>
  )
}

/** CheckDetail renders a check's actionable next step: the CTA message plus a copyable fix command, when either exists. Renders nothing otherwise. */
export function CheckDetail({ check }: { check: DoctorCheck }) {
  const cta = getCheckCta(check)
  if (!cta && !check.fix) return null
  return (
    <div className="mt-2 space-y-2">
      {cta ? (
        <div className="rounded-md bg-muted/50 p-2.5">
          <p className="text-xs text-muted-foreground">{cta.message}</p>
          <div className="mt-1.5">{cta.action}</div>
        </div>
      ) : null}
      {check.fix ? (
        <FixCommand fix={check.fix} docsPath={check.docs_path} />
      ) : null}
    </div>
  )
}
