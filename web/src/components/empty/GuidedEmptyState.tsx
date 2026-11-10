import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { CodeBlock } from '@/components/ui/code-block'
import { EmptyState } from '@/components/ui/empty-state'
import { useBrand } from '@/hooks/useBrand'

/** GuidedEmptyState is the first-run state of a list: one primary action plus the matching CLI command. */
export function GuidedEmptyState({
  total,
  icon,
  title,
  description,
  action,
  cliArgs,
  helpPath,
}: {
  // The unfiltered total: a filter that hides every row is FilterChips' case, not this one.
  total: number
  icon: ReactNode
  title: string
  description: string
  action: ReactNode
  cliArgs: string
  helpPath?: string
}) {
  const { t } = useTranslation('uiShared')
  const brand = useBrand()
  if (total > 0) return null
  return (
    <section className="flex flex-col gap-2" aria-label={title}>
      <EmptyState
        kind="first-run"
        icon={icon}
        title={title}
        description={description}
        action={action}
        helpPath={helpPath}
      />
      <details className="group rounded-glinr-group border border-border px-3 py-2 text-sm">
        <summary className="cursor-pointer text-muted-foreground select-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-state-accent focus-visible:outline-none">
          {t('guidedEmpty.viaCli')}
        </summary>
        <CodeBlock
          className="mt-2 w-full text-left"
          code={`${brand.BinaryName} ${cliArgs}`}
          language="bash"
          label={t('guidedEmpty.cliLabel')}
        />
      </details>
    </section>
  )
}
