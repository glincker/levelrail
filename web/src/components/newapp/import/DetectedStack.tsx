import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { stackLabel } from '../../../lib/repoImport'
import type { ImportAnalysis } from '../../../queries/importAnalyze'

/** One calm line about what was detected, plus the monorepo app choice. */
export function DetectedStack({
  analysis,
  baseDirectory,
  onBaseDirectory,
  onEdit,
}: {
  analysis: ImportAnalysis
  baseDirectory: string
  onBaseDirectory: (dir: string) => void
  onEdit: () => void
}) {
  const { t } = useTranslation('newApp')
  const packages = analysis.packages ?? []
  const builder = t(`import.builders.${analysis.build.type}`, {
    defaultValue: analysis.build.type,
  })
  return (
    <div className="min-w-0 space-y-3 rounded-md border border-border bg-muted/30 p-3">
      <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
        <p className="min-w-0 text-sm text-foreground">
          {t('import.detected.line', {
            stack: stackLabel(analysis),
            builder,
            port: analysis.port.value,
          })}
        </p>
        <Button size="sm" variant="ghost" onClick={onEdit}>
          {t('import.detected.edit')}
        </Button>
      </div>
      {analysis.monorepo && packages.length > 1 ? (
        <div className="space-y-1">
          <p className="text-xs text-muted-foreground">
            {t('import.detected.monorepo')}
          </p>
          <Select
            value={baseDirectory || (packages[0]?.dir ?? '')}
            onValueChange={(v) => {
              if (v) onBaseDirectory(v)
            }}
          >
            <SelectTrigger
              className="w-full min-w-0 sm:w-72"
              aria-label={t('import.detected.monorepo')}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {packages.map((p) => (
                <SelectItem key={p.dir} value={p.dir}>
                  {p.name} ({p.dir})
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ) : null}
    </div>
  )
}
