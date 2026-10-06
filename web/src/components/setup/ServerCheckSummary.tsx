import { useState } from 'react'
import {
  CaretDownIcon,
  CheckCircleIcon,
  WarningCircleIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsiblePanel,
} from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'
import { CheckDetail } from '../CheckDetail'
import { HostFirewallToggle } from '../HostFirewallToggle'
import { groupServerChecks } from '../../lib/setupWizard'
import type { DoctorCheck, DoctorReport } from '../../queries/systemDoctor'

/** CheckRow is one collapsed line: a name and a severity icon that expands into the real message, the fix command, and a docs link. Blocking checks start expanded since there are usually only one or two and they need action now; optional ones start collapsed. */
function CheckRow({
  check,
  tone,
}: {
  check: DoctorCheck
  tone: 'blocking' | 'attention'
}) {
  const [open, setOpen] = useState(tone === 'blocking')
  const Icon = tone === 'blocking' ? XCircleIcon : WarningCircleIcon
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="rounded-md border border-border"
    >
      <CollapsibleTrigger className="flex w-full items-start gap-2.5 px-2.5 py-2 text-left">
        <Icon
          className={cn(
            'mt-0.5 size-4 shrink-0',
            tone === 'blocking'
              ? 'text-destructive'
              : 'text-amber-600 dark:text-amber-400',
          )}
          aria-hidden="true"
        />
        <span className="min-w-0 flex-1 text-sm font-medium text-foreground">
          {check.name}
        </span>
        <CaretDownIcon
          className={cn(
            'mt-0.5 size-3.5 shrink-0 text-muted-foreground transition-transform motion-reduce:transition-none',
            open && 'rotate-180',
          )}
          aria-hidden="true"
        />
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <div className="px-2.5 pb-2.5 pl-9">
          <p className="text-xs text-muted-foreground">{check.message}</p>
          <CheckDetail check={check} />
          {check.code === 'firewall' ? (
            <div className="mt-2">
              <HostFirewallToggle />
            </div>
          ) : null}
        </div>
      </CollapsiblePanel>
    </Collapsible>
  )
}

/** ServerCheckSummary is the setup wizard's compact view of the doctor bundle: a plain-language count of what needs attention, grouped by urgency, each check collapsed until the operator wants the detail. A healthy check takes up a single line in a collapsed list, not the same space as a warning. */
export function ServerCheckSummary({ report }: { report: DoctorReport }) {
  const [passedOpen, setPassedOpen] = useState(false)
  const { blocking, attention, passed } = groupServerChecks(report)

  if (blocking.length === 0 && attention.length === 0) {
    return (
      <Alert>
        <CheckCircleIcon className="text-green-600 dark:text-green-400" />
        <AlertTitle>This server is ready</AlertTitle>
        <AlertDescription>
          All {passed.length} checks passed. Nothing to fix before you continue.
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="space-y-4">
      {blocking.length > 0 ? (
        <div className="space-y-2">
          <Alert variant="destructive">
            <XCircleIcon />
            <AlertTitle>
              {blocking.length === 1
                ? 'One thing to fix before you can deploy'
                : `${blocking.length} things to fix before you can deploy`}
            </AlertTitle>
            <AlertDescription>
              Nothing can run on this server until these pass. Fix one, then
              re-run the checks.
            </AlertDescription>
          </Alert>
          <div className="space-y-1.5">
            {blocking.map((check) => (
              <CheckRow key={check.code} check={check} tone="blocking" />
            ))}
          </div>
        </div>
      ) : null}

      {attention.length > 0 ? (
        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">
            <span className="font-medium text-foreground">
              {attention.length} {attention.length === 1 ? 'thing' : 'things'}{' '}
              worth a look
            </span>{' '}
            when you have a minute. These are optional and will not stop you
            from deploying.
          </p>
          <div className="space-y-1.5">
            {attention.map((check) => (
              <CheckRow key={check.code} check={check} tone="attention" />
            ))}
          </div>
        </div>
      ) : null}

      {passed.length > 0 ? (
        <Collapsible open={passedOpen} onOpenChange={setPassedOpen}>
          <CollapsibleTrigger className="group flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground">
            <CheckCircleIcon
              className="size-3.5 text-green-600 dark:text-green-400"
              aria-hidden="true"
            />
            {passed.length} other {passed.length === 1 ? 'check' : 'checks'}{' '}
            passed
            <CaretDownIcon
              className="size-3 transition-transform group-data-[panel-open]:rotate-180 motion-reduce:transition-none"
              aria-hidden="true"
            />
          </CollapsibleTrigger>
          <CollapsiblePanel>
            <ul className="mt-1.5 space-y-1 pl-5 text-xs text-muted-foreground">
              {passed.map((check) => (
                <li key={check.code}>{check.name}</li>
              ))}
            </ul>
          </CollapsiblePanel>
        </Collapsible>
      ) : null}
    </div>
  )
}
