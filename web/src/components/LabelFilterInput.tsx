import { useTranslation } from 'react-i18next'
import { TagGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'

export function LabelFilterInput({
  value,
  onChange,
  invalid,
}: {
  value: string
  onChange: (next: string) => void
  invalid?: boolean
}) {
  const { t } = useTranslation('common')
  return (
    <div className="space-y-1">
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t('resourceLabels.filterPlaceholder')}
        aria-label={t('resourceLabels.filterLabel')}
        aria-invalid={invalid ? true : undefined}
        className="max-w-xs font-mono text-xs placeholder:text-muted-foreground/70"
      />
      {invalid ? (
        <p className="text-xs text-destructive">
          {t('resourceLabels.filterInvalid')}
        </p>
      ) : null}
    </div>
  )
}

/** LabelFilterField folds the label selector into a Labels popover so the filter row shows no stray selector text. */
export function LabelFilterField(props: {
  value: string
  onChange: (next: string) => void
  invalid?: boolean
}) {
  const { t } = useTranslation('common')
  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            type="button"
            variant={props.value ? 'default' : 'outline'}
            size="sm"
            aria-label={t('resourceLabels.filterLabel')}
          />
        }
      >
        <TagGlassIcon size="1.4em" aria-hidden="true" />
        <span className="max-sm:sr-only">
          {t('resourceLabels.filterButton')}
        </span>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 space-y-1.5 p-2">
        <p className="text-xs font-medium text-foreground">
          {t('resourceLabels.filterLabel')}
        </p>
        <LabelFilterInput {...props} />
      </PopoverContent>
    </Popover>
  )
}
