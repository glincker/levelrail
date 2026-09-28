import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { StatusPill, type Tone } from '@/components/kit'
import { Button } from '../ui/button'
import {
  preflightQueryOptions,
  type UpgradeCheckStatus,
} from '../../queries/updates'

const STATUS_TONE: Record<UpgradeCheckStatus, Tone> = {
  ok: 'success',
  warn: 'warning',
  fail: 'danger',
  unknown: 'neutral',
}

const STATUS_LABEL: Record<UpgradeCheckStatus, string> = {
  ok: 'Pass',
  warn: 'Warning',
  fail: 'Blocked',
  unknown: 'Unknown',
}

function CopyCommand({ label, command }: { label: string; command: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="space-y-1.5">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <div className="flex items-center gap-2">
        <code className="min-w-0 flex-1 overflow-x-auto rounded-md bg-muted px-3 py-2 font-mono text-xs whitespace-nowrap">
          {command}
        </code>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            void navigator.clipboard.writeText(command).then(() => {
              setCopied(true)
              setTimeout(() => {
                setCopied(false)
              }, 2000)
            })
          }}
        >
          {copied ? <CheckIcon /> : <CopyIcon />}
          {copied ? 'Copied' : 'Copy'}
        </Button>
      </div>
    </div>
  )
}

export function UpgradePreflight() {
  const { data, isPending, isError } = useQuery(preflightQueryOptions())
  if (isPending) {
    return (
      <p className="text-sm text-muted-foreground">
        Running preflight checks...
      </p>
    )
  }
  if (isError) {
    return (
      <p className="text-sm text-muted-foreground">
        Preflight checks are unavailable right now.
      </p>
    )
  }
  return (
    <div className="space-y-4">
      {data.release_notes ? (
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">
            Release notes
          </p>
          <pre className="max-h-48 overflow-auto rounded-md bg-muted px-3 py-2 font-sans text-xs whitespace-pre-wrap">
            {data.release_notes}
          </pre>
        </div>
      ) : null}
      <ul className="space-y-2" aria-label="Upgrade preflight checks">
        {data.checks.map((c) => (
          <li key={c.code} className="flex items-start gap-3 text-sm">
            <StatusPill
              tone={STATUS_TONE[c.status]}
              label={STATUS_LABEL[c.status]}
              size="sm"
            />
            <span>
              <span className="font-medium text-foreground">{c.name}</span>
              <span className="block text-xs text-muted-foreground">
                {c.message}
              </span>
            </span>
          </li>
        ))}
      </ul>
      {data.blocked ? (
        <p className="text-sm text-destructive">
          Upgrade blocked: fix the failing checks first.
        </p>
      ) : data.update_available ? (
        <div className="space-y-3">
          <CopyCommand
            label="Run this on the server (never runs automatically)"
            command={data.upgrade_command}
          />
          <CopyCommand
            label="To roll back the database after a bad upgrade"
            command={data.rollback_command}
          />
        </div>
      ) : null}
    </div>
  )
}
