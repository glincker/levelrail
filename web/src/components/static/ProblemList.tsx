import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import type { StaticRouteProblem } from '../../queries/staticRoutes'

export function ProblemList({
  problems,
}: Readonly<{ problems: StaticRouteProblem[] }>) {
  const { t } = useTranslation('staticRoutes')
  return (
    <ul className="grid gap-1.5 text-sm">
      {problems.map((p) => (
        <li
          key={`${p.source}:${p.line ?? 0}:${p.message}`}
          className="flex items-start gap-2"
        >
          <TriangleAlertGlassIcon
            className="mt-0.5 size-6 shrink-0 text-tone-warning"
            aria-hidden="true"
          />
          <span>
            <span className="font-mono text-xs text-muted-foreground">
              {p.source}
              {p.line ? ` ${t('problems.line', { line: p.line })}` : ''}
            </span>{' '}
            {p.message}
          </span>
        </li>
      ))}
    </ul>
  )
}
