import { useTranslation } from 'react-i18next'
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
  const { t } = useTranslation('gitSource')
  return (
    <Field>
      <FieldLabel>{t('services.additionalLabel')}</FieldLabel>
      <FieldDescription>{t('services.additionalDescription')}</FieldDescription>
      <div className="space-y-2">
        {rows.map((row, index) => (
          <GitSourceServiceRow
            key={index}
            row={row}
            namePlaceholder="worker"
            pathPlaceholder="./worker/Dockerfile"
            withPort={false}
            disabled={disabled}
            removeLabel={t('services.removeAdditional')}
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
          {t('services.add')}
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
  const { t } = useTranslation('gitSource')
  return (
    <Field>
      <FieldLabel>{t('services.mapLabel')}</FieldLabel>
      <FieldDescription>
        {t('services.mapDescription', { appName })}
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
                removeLabel={t('services.remove')}
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
                  {t('services.duplicate')}
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
          {t('services.add')}
        </Button>
        {checking ? (
          <p className="text-xs text-muted-foreground" aria-live="polite">
            {t('services.checking')}
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
