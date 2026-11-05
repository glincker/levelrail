import { useId, useState } from 'react'
import { WrenchGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { formatChangeValue } from '../lib/diagnosisFix'
import { useApplyDiagnosisFix, useDiagnosis } from '../queries/diagnosis'
import type { DiagnosisCause, DiagnosisFix } from '../types/diagnosis'

// Typed causes from the deterministic diagnosis, each with evidence and
// fixes. A fix is previewed as a field diff and applied through the normal
// app update, so the caller's own permissions and the audit log apply.
export function DiagnosisFixes({
  appName,
  deployId,
  fixesOnly = false,
}: {
  appName: string
  deployId?: string
  // Embedded in a card that already shows the cause and evidence.
  fixesOnly?: boolean
}) {
  const { data } = useDiagnosis(appName, deployId, true)
  const causes = data?.causes ?? []
  if (causes.length === 0) {
    return null
  }
  return (
    <div className="space-y-3" data-testid="diagnosis-causes">
      {causes.map((cause) => (
        <CauseCard
          key={cause.code}
          appName={appName}
          cause={cause}
          fixesOnly={fixesOnly}
        />
      ))}
    </div>
  )
}

function CauseCard({
  appName,
  cause,
  fixesOnly,
}: {
  appName: string
  cause: DiagnosisCause
  fixesOnly: boolean
}) {
  const fixes = (
    <ul className={fixesOnly ? 'space-y-2' : 'mt-3 space-y-2'}>
      {cause.fixes.map((fix) => (
        <li key={fix.n}>
          <FixRow appName={appName} fix={fix} />
        </li>
      ))}
    </ul>
  )
  if (fixesOnly) return fixes
  return (
    <div className="rounded-lg border border-border p-3 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <p className="font-medium text-foreground">{cause.title}</p>
        <Badge
          variant={cause.confidence === 'high' ? 'destructive' : 'warning'}
        >
          {cause.confidence === 'high' ? 'High confidence' : 'Possible cause'}
        </Badge>
      </div>
      <p className="mt-1 text-muted-foreground">{cause.explanation}</p>
      {cause.evidence.length > 0 ? (
        <ul className="mt-2 space-y-1">
          {cause.evidence.map((e, i) => (
            <li
              key={`${e.source}-${i}`}
              className="truncate font-mono text-xs text-muted-foreground"
              title={e.excerpt}
            >
              [{e.source}] {e.excerpt}
            </li>
          ))}
        </ul>
      ) : null}
      {fixes}
    </div>
  )
}

const URL_RE = /(https?:\/\/[^\s)]+)/g

// Renders a fix hint with real links, so a terms URL can be read before applying.
function Hint({ text }: { text: string }) {
  return (
    <p className="text-xs text-muted-foreground">
      {text.split(URL_RE).map((part, i) =>
        i % 2 === 1 ? (
          <a
            key={`${part}-${i}`}
            href={part}
            target="_blank"
            rel="noreferrer"
            className="break-all text-foreground underline"
          >
            {part}
          </a>
        ) : (
          part
        ),
      )}
    </p>
  )
}

function FixRow({ appName, fix }: { appName: string; fix: DiagnosisFix }) {
  const [open, setOpen] = useState(false)
  const [inputs, setInputs] = useState<Record<string, string>>({})
  const [redeploy, setRedeploy] = useState(true)
  const redeployId = useId()
  const apply = useApplyDiagnosisFix(appName)

  if (fix.kind === 'manual') {
    return (
      <div className="rounded bg-muted/50 px-2 py-1.5">
        <p className="text-foreground">{fix.label}</p>
        {fix.hint ? <Hint text={fix.hint} /> : null}
      </div>
    )
  }

  const missingInput = fix.changes.some(
    (c) => c.needs_input && !(inputs[c.field] ?? '').trim(),
  )
  const onApply = () => {
    apply.mutate(
      { fix, inputs, redeploy: redeploy && Boolean(fix.redeploy) },
      {
        onSuccess: () => {
          toast.add({ title: 'Fix applied.', type: 'success' })
          setOpen(false)
        },
        onError: (error) => toast.add({ title: error.message, type: 'error' }),
      },
    )
  }

  return (
    <div className="rounded bg-muted/50 px-2 py-1.5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-foreground">{fix.label}</p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
        >
          <WrenchGlassIcon size="1.4em" aria-hidden="true" />
          {open ? 'Hide preview' : 'Preview fix'}
        </Button>
      </div>
      {fix.hint ? <Hint text={fix.hint} /> : null}
      {open ? (
        <div className="mt-2 space-y-2">
          {fix.changes.map((c) => (
            <div key={c.field} className="space-y-1">
              <p className="break-all font-mono text-xs text-foreground">
                {c.field}
              </p>
              {c.needs_input ? (
                <Input
                  aria-label={`Value for ${c.field}`}
                  placeholder="Value"
                  value={inputs[c.field] ?? ''}
                  onChange={(e) =>
                    setInputs((prev) => ({
                      ...prev,
                      [c.field]: e.target.value,
                    }))
                  }
                />
              ) : (
                <p className="break-all font-mono text-xs text-muted-foreground">
                  {formatChangeValue(c.field, c.from)} {'->'}{' '}
                  {formatChangeValue(c.field, c.to)}
                </p>
              )}
            </div>
          ))}
          {fix.redeploy ? (
            <label
              htmlFor={redeployId}
              className="flex items-center gap-2 text-xs text-foreground"
            >
              <Checkbox
                id={redeployId}
                checked={redeploy}
                onCheckedChange={(v) => setRedeploy(v === true)}
              />
              Redeploy after applying
            </label>
          ) : null}
          <Button
            type="button"
            size="sm"
            disabled={apply.isPending || missingInput}
            onClick={onApply}
          >
            {apply.isPending ? 'Applying...' : 'Apply fix'}
          </Button>
        </div>
      ) : null}
    </div>
  )
}
