import { PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import type { SpecValidationIssue } from '../queries/specValidation'
import type { AdditionalServiceRow, ServiceSpecRow } from '../lib/gitSourceForm'
import { GitSourceServiceRow } from './GitSourceServiceRow'

function errorsFor(message: string | null) {
  return message ? [{ message }] : undefined
}

export function AdditionalServicesField({
  rows,
  disabled,
  error,
  onAdd,
  onUpdate,
  onRemove,
}: {
  rows: AdditionalServiceRow[]
  disabled: boolean
  error: string | null
  onAdd: () => void
  onUpdate: (index: number, patch: Partial<AdditionalServiceRow>) => void
  onRemove: (index: number) => void
}) {
  return (
    <Field>
      <FieldLabel>{/* i18n-ignore */}Additional services (optional)</FieldLabel>
      <FieldDescription>
        Also rebuild these sibling services from the same push, for a monorepo
        with more than one service. Each needs its own already-existing service
        name and build config. Saving with any row filled in always uses the
        direct git-source connection, even for a picked provider repo, since
        automatic webhook registration doesn&apos;t support monorepo fan-out
        yet.
      </FieldDescription>
      <div className="space-y-2">
        {rows.map((row, index) => (
          <GitSourceServiceRow
            key={index}
            row={row}
            namePlaceholder="worker"
            pathPlaceholder="./worker/Dockerfile"
            withPort={false}
            disabled={disabled}
            removeLabel="Remove additional service"
            onChange={(patch) => {
              onUpdate(index, patch)
            }}
            onRemove={() => {
              onRemove(index)
            }}
          />
        ))}
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={disabled}
          onClick={onAdd}
        >
          <PlusGlassIcon size="1.4em" />
          Add service
        </Button>
      </div>
      <FieldError errors={errorsFor(error)} />
    </Field>
  )
}

export function ServicesMapField({
  appName,
  rows,
  disabled,
  error,
  duplicateNames,
  issuesForService,
  unmatchedIssues,
  checking,
  onAdd,
  onUpdate,
  onRemove,
}: {
  appName: string
  rows: ServiceSpecRow[]
  disabled: boolean
  error: string | null
  duplicateNames: Set<string>
  issuesForService: (name: string) => SpecValidationIssue[]
  unmatchedIssues: SpecValidationIssue[]
  checking: boolean
  onAdd: () => void
  onUpdate: (index: number, patch: Partial<ServiceSpecRow>) => void
  onRemove: (index: number) => void
}) {
  return (
    <Field>
      <FieldLabel>{/* i18n-ignore */}Services (app.yaml-style)</FieldLabel>
      <FieldDescription>
        Every push fans out into an independent build and deploy per service,
        the same POST .../deploy-spec fan-out a manual multi-service deploy
        uses, under one app named <span className="font-mono">{appName}</span>.
        Replaces the single-service build above; each service key becomes its
        own real service named{' '}
        <span className="font-mono">{appName}-&lt;key&gt;</span>.
      </FieldDescription>
      <div className="space-y-2">
        {rows.map((row, index) => {
          const rowName = row.serviceName.trim()
          const rowIssues = rowName ? issuesForService(rowName) : []
          const isDuplicate = rowName !== '' && duplicateNames.has(rowName)
          const hasRowProblem = isDuplicate || rowIssues.length > 0
          const rowErrorId = `service-row-error-${index}`
          return (
            <div key={index} className="space-y-1">
              <GitSourceServiceRow
                row={row}
                namePlaceholder="web"
                pathPlaceholder="./web/Dockerfile"
                withPort
                disabled={disabled}
                invalid={hasRowProblem}
                describedBy={hasRowProblem ? rowErrorId : undefined}
                removeLabel="Remove service"
                onChange={(patch) => {
                  onUpdate(index, patch)
                }}
                onRemove={() => {
                  onRemove(index)
                }}
              />
              {isDuplicate ? (
                <p
                  id={rowErrorId}
                  role="alert"
                  className="text-xs text-destructive"
                >
                  Another row already uses this service name.
                </p>
              ) : rowIssues.length > 0 ? (
                <ul id={rowErrorId} role="alert" className="space-y-0.5">
                  {rowIssues.map((issue, i) => (
                    <li key={i} className="text-xs text-destructive">
                      {issue.message}
                    </li>
                  ))}
                </ul>
              ) : null}
            </div>
          )
        })}
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={disabled}
          onClick={onAdd}
        >
          <PlusGlassIcon size="1.4em" />
          Add service
        </Button>
        {checking ? (
          <p className="text-xs text-muted-foreground" aria-live="polite">
            Checking against the app spec schema...
          </p>
        ) : null}
        {unmatchedIssues.length > 0 ? (
          <ul className="space-y-0.5" role="alert">
            {unmatchedIssues.map((issue, i) => (
              <li key={i} className="text-xs text-destructive">
                {issue.path ? `${issue.path}: ` : ''}
                {issue.message}
              </li>
            ))}
          </ul>
        ) : null}
      </div>
      <FieldError errors={errorsFor(error)} />
    </Field>
  )
}
