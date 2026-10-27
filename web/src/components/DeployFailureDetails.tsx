import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CaretRightIcon } from '@phosphor-icons/react/dist/ssr'
import type { DeployFailure } from '../types/deployFailure'
import { CodeBlock } from '@/components/ui/code-block'

// Codes are snake_case slugs; a readable label needs no per-code table.
function failureClassLabel(code: string): string {
  const words = code.replace(/_/g, ' ').trim()
  return words.charAt(0).toUpperCase() + words.slice(1)
}

/** Failure class, failing step and a collapsible, copyable error excerpt for a deploy attempt. */
export function DeployFailureDetails({ failure }: { failure?: DeployFailure }) {
  const { t } = useTranslation('gitBuild')
  const [open, setOpen] = useState(false)
  const excerptId = useId()

  if (!failure?.code && !failure?.log_excerpt) return null

  return (
    <div className="mt-1.5 space-y-1.5">
      <p className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
        {failure.code ? (
          <span className="rounded border border-tone-danger-border px-1.5 py-0.5 font-medium text-tone-danger">
            <span className="sr-only">{t('failureDetails.classLabel')}: </span>
            {failureClassLabel(failure.code)}
          </span>
        ) : null}
        {failure.failing_step ? (
          <span>
            {t('failureDetails.failedAt', { step: failure.failing_step })}
          </span>
        ) : null}
      </p>
      {failure.log_excerpt ? (
        <div>
          <button
            type="button"
            aria-expanded={open}
            aria-controls={excerptId}
            onClick={() => setOpen((v) => !v)}
            className="flex items-center gap-1 rounded text-xs font-medium text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2"
          >
            <CaretRightIcon
              aria-hidden="true"
              className={`size-3 transition-transform motion-reduce:transition-none ${open ? 'rotate-90' : ''}`}
            />
            {t('failureDetails.excerpt')}
          </button>
          <div id={excerptId} hidden={!open} className="mt-1">
            {open ? (
              <CodeBlock
                code={failure.log_excerpt}
                label={t('failureDetails.excerpt')}
                wrap
                wrapToggle={false}
                maxHeight="sm"
              />
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  )
}
