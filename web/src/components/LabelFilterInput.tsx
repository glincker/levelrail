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
        className="max-w-xs font-mono text-xs"
      />
      {invalid ? (
        <p className="text-xs text-destructive">
          {t('resourceLabels.filterInvalid')}
        </p>
      ) : null}
    </div>
  )
}

/** On phones the label selector folds into a popover so the filter row stays one line. */
export function LabelFilterField(props: {
  value: string
  onChange: (next: string) => void
  invalid?: boolean
}) {
  const { t } = useTranslation('common')
  return (
    <>
      <div className="sm:hidden">
        <Popover>
          <PopoverTrigger
            render={
              <Button
                type="button"
                variant={props.value ? 'default' : 'outline'}
                size="icon-sm"
                aria-label={t('resourceLabels.filterLabel')}
              />
            }
          >
            <TagGlassIcon size="1.4em" aria-hidden="true" />
          </PopoverTrigger>
          <PopoverContent align="end" className="w-72 p-2">
            <LabelFilterInput {...props} />
          </PopoverContent>
        </Popover>
      </div>
      <div className="hidden w-72 sm:block">
        <LabelFilterInput {...props} />
      </div>
    </>
  )
}
