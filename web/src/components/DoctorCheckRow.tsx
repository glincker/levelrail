import { useState } from 'react'
import {
  CheckCircleIcon,
  WarningCircleIcon,
  XCircleIcon,
  MinusCircleIcon,
  CopyIcon,
  CheckIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { VariantProps } from 'class-variance-authority'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { HelpLink } from '@/components/HelpLink'
import type { DoctorCheck, DoctorCheckStatus } from '../queries/systemDoctor'
import { getCheckCta } from './doctorCheckCtas'

const STATUS_META: Record<
  DoctorCheckStatus,
  {
    label: string
    variant: VariantProps<typeof badgeVariants>['variant']
    icon: Icon
  }
> = {
  ok: { label: 'OK', variant: 'success', icon: CheckCircleIcon },
  warn: { label: 'Warning', variant: 'warning', icon: WarningCircleIcon },
  fail: { label: 'Failed', variant: 'destructive', icon: XCircleIcon },
  unknown: { label: 'Not checked', variant: 'muted', icon: MinusCircleIcon },
}

// FixCommand renders a check's backend-supplied Fix as a copyable code
// block plus a HelpLink to its DocsPath, the CLI's own printSystemDoctorFixes
// (cmd/levelrail-cli/output.go) rendered as a table row instead: same
// two fields, different surface.
function FixCommand({ fix, docsPath }: { fix: string; docsPath?: string }) {
  const [copied, setCopied] = useState(false)

  function copyFix() {
    void navigator.clipboard.writeText(fix).then(() => {
      setCopied(true)
    })
  }

  return (
    <div className="mt-2 rounded-md bg-muted/50 p-2.5">
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

export function DoctorCheckRow({ check }: { check: DoctorCheck }) {
  const meta = STATUS_META[check.status]
  const StatusIcon = meta.icon
  const cta = getCheckCta(check)
  return (
    <div className="py-2.5 text-sm">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-medium text-foreground">{check.name}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {check.message}
          </p>
        </div>
        <Badge variant={meta.variant} className="shrink-0">
          <StatusIcon />
          {meta.label}
        </Badge>
      </div>
      {cta ? (
        <div className="mt-2 rounded-md bg-muted/50 p-2.5">
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
