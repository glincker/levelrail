import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import {
  CheckCircleIcon,
  FloppyDiskIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import {
  usePipeline,
  useSavePipeline,
  useValidatePipeline,
} from '../queries/pipelines'
import { lineOffsets, NEW_PIPELINE_YAML } from '../lib/pipelineTemplate'
import { PipelineTriggerFilters } from './PipelineTriggerFilters'
import type { PipelineIssue } from '../types/pipelines'

const LINE_HEIGHT_CLASS = 'leading-5'

function IssueList({
  issues,
  onJump,
}: {
  issues: PipelineIssue[]
  onJump: (line: number) => void
}) {
  return (
    <ul className="space-y-1 text-sm" aria-label="Validation problems">
      {issues.map((i, n) => (
        <li key={n}>
          <button
            type="button"
            onClick={() => onJump(i.line)}
            className="flex w-full items-start gap-2 rounded-md px-2 py-1 text-left text-destructive hover:bg-destructive/10"
          >
            <WarningCircleIcon
              className="mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />
            <span>
              {i.line > 0 ? (
                <span className="font-mono">line {i.line}: </span>
              ) : null}
              {i.message}
            </span>
          </button>
        </li>
      ))}
    </ul>
  )
}

function YamlBox({
  value,
  onChange,
  errorLines,
  boxRef,
}: {
  value: string
  onChange: (next: string) => void
  errorLines: Set<number>
  boxRef: React.RefObject<HTMLTextAreaElement | null>
}) {
  const gutterRef = useRef<HTMLDivElement>(null)
  const count = value.split('\n').length
  return (
    <div className="flex h-[28rem] overflow-hidden rounded-lg border border-input font-mono text-xs">
      <div
        ref={gutterRef}
        aria-hidden="true"
        className={cn(
          'w-10 shrink-0 select-none overflow-hidden bg-muted py-2 pr-2 text-right text-muted-foreground',
          LINE_HEIGHT_CLASS,
        )}
      >
        {Array.from({ length: count }, (_, i) => (
          <div
            key={i}
            className={
              errorLines.has(i + 1) ? 'font-bold text-destructive' : ''
            }
          >
            {i + 1}
          </div>
        ))}
      </div>
      <textarea
        ref={boxRef}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onScroll={(e) => {
          if (gutterRef.current) {
            gutterRef.current.scrollTop = e.currentTarget.scrollTop
          }
        }}
        spellCheck={false}
        wrap="off"
        aria-label="Pipeline YAML"
        className={cn(
          'min-w-0 flex-1 resize-none overflow-auto bg-transparent px-2 py-2 outline-none focus-visible:ring-2 focus-visible:ring-ring/50',
          LINE_HEIGHT_CLASS,
        )}
      />
    </div>
  )
}

export function PipelineEditor({
  appName,
  pipelineName,
}: {
  appName: string
  pipelineName?: string
}) {
  const existing = usePipeline(appName, pipelineName ?? '')
  const isEdit = Boolean(pipelineName)
  const [name, setName] = useState('')
  const [yaml, setYaml] = useState<string | null>(
    isEdit ? null : NEW_PIPELINE_YAML,
  )
  const boxRef = useRef<HTMLTextAreaElement>(null)
  const save = useSavePipeline(appName)
  const validate = useValidatePipeline()
  const navigate = useNavigate()

  const text = yaml ?? existing.data?.yaml ?? ''
  const debounced = useDebouncedValue(text, 400)
  const { mutate: runValidate } = validate
  useEffect(() => {
    if (debounced.trim() !== '') {
      runValidate(debounced)
    }
  }, [debounced, runValidate])

  if (isEdit && existing.isLoading) {
    return <Skeleton className="h-96 w-full" />
  }
  if (isEdit && existing.error) {
    return <p className="text-sm text-destructive">{existing.error.message}</p>
  }

  const issues = validate.data?.issues ?? []
  const settled = debounced === text && validate.isSuccess
  const valid = settled && validate.data?.valid === true
  const errorLines = new Set(issues.map((i) => i.line).filter((l) => l > 0))

  const jump = (line: number) => {
    const box = boxRef.current
    if (!box || line < 1) {
      return
    }
    const [start, end] = lineOffsets(text, line)
    box.focus()
    box.setSelectionRange(start, end)
    box.scrollTop = Math.max(0, (line - 3) * 20)
  }

  const submit = () =>
    save.mutate(
      {
        existing: pipelineName,
        req: { name: pipelineName ? undefined : name || undefined, yaml: text },
      },
      {
        onSuccess: () => {
          toast.add({ title: 'Pipeline saved.', type: 'success' })
          void navigate({
            to: '/apps/$name/pipelines',
            params: { name: appName },
          })
        },
        onError: (e) => toast.add({ title: e.message, type: 'error' }),
      },
    )

  return (
    <section className="space-y-4 rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold text-foreground">
            {isEdit ? `Edit ${pipelineName}` : 'New pipeline'}
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            <Link
              to="/apps/$name/pipelines"
              params={{ name: appName }}
              className="underline underline-offset-2"
            >
              Back to pipelines
            </Link>
          </p>
        </div>
        {!isEdit ? (
          <div className="space-y-1.5">
            <Label htmlFor="pipeline-name">
              Name (defaults to the name in the file)
            </Label>
            <Input
              id="pipeline-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-56"
            />
          </div>
        ) : null}
      </div>
      <YamlBox
        value={text}
        onChange={setYaml}
        errorLines={errorLines}
        boxRef={boxRef}
      />
      {valid && validate.data?.filters ? (
        <PipelineTriggerFilters
          key={JSON.stringify(validate.data.filters)}
          yaml={text}
          filters={validate.data.filters}
          onApply={setYaml}
        />
      ) : null}
      <div aria-live="polite" className="text-sm">
        {valid ? (
          <p className="flex items-center gap-1.5 text-green-700 dark:text-green-400">
            <CheckCircleIcon className="size-4" aria-hidden="true" />
            Valid
            {validate.data?.jobs ? `, ${validate.data.jobs} jobs` : ''}
          </p>
        ) : issues.length > 0 ? (
          <IssueList issues={issues} onJump={jump} />
        ) : (
          <p className="text-muted-foreground">Checking...</p>
        )}
      </div>
      <Button onClick={submit} disabled={!valid || save.isPending}>
        <FloppyDiskIcon aria-hidden="true" />
        {save.isPending ? 'Saving...' : 'Save pipeline'}
      </Button>
    </section>
  )
}
