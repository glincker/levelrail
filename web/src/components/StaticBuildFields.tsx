import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { BrandIcon, isBrandIconName } from './BrandIcon'
import { staticBuildMismatch, type StaticBuildValue } from '../lib/staticBuild'
import type { StaticBuildSuggestion } from '../types/gitSource'

interface StaticBuildFieldsProps {
  value: StaticBuildValue
  suggestion?: StaticBuildSuggestion
  disabled: boolean
  onChange: (next: StaticBuildValue) => void
}

export function StaticBuildFields({
  value,
  suggestion,
  disabled,
  onChange,
}: StaticBuildFieldsProps) {
  const { t } = useTranslation('gitBuild')
  const alreadyApplied =
    suggestion !== undefined &&
    value.command === suggestion.command &&
    value.publish === suggestion.publish_dir
  const icon = suggestion?.preset_id.replace(/-static$/, '') ?? ''
  return (
    <div className="space-y-3">
      {suggestion && !alreadyApplied ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-input p-2">
          <span className="flex items-center gap-2 text-sm">
            {isBrandIconName(icon) ? (
              <BrandIcon name={icon} className="size-4 shrink-0" />
            ) : null}
            {t('staticBuild.detected', { name: suggestion.preset_name })}
          </span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={disabled}
            onClick={() => {
              onChange({
                command: suggestion.command,
                publish: suggestion.publish_dir,
              })
            }}
          >
            {t('staticBuild.use')}
          </Button>
        </div>
      ) : null}
      <Field>
        <FieldLabel htmlFor="git-build-command">
          {t('staticBuild.command')}
        </FieldLabel>
        <Input
          id="git-build-command"
          className="font-mono"
          placeholder={t('staticBuild.commandPlaceholder')}
          autoComplete="off"
          spellCheck={false}
          value={value.command}
          disabled={disabled}
          onChange={(e) => {
            onChange({ ...value, command: e.target.value })
          }}
        />
        <FieldDescription>{t('staticBuild.commandHelp')}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel htmlFor="git-build-publish">
          {t('staticBuild.publish')}
        </FieldLabel>
        <Input
          id="git-build-publish"
          className="font-mono"
          placeholder={t('staticBuild.publishPlaceholder')}
          autoComplete="off"
          spellCheck={false}
          value={value.publish}
          disabled={disabled}
          onChange={(e) => {
            onChange({ ...value, publish: e.target.value })
          }}
        />
        <FieldDescription>
          {staticBuildMismatch(value)
            ? t('staticBuild.mismatch')
            : t('staticBuild.publishHelp')}
        </FieldDescription>
      </Field>
    </div>
  )
}
