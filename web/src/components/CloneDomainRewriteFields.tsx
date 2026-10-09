import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'

import type { CloneDomainRewrite } from '../lib/cloneDomainRewrite'

// Copy every app's domain sets onto its clone under rewritten hostnames.
export function CloneDomainRewriteFields({
  value,
  onChange,
}: {
  value: CloneDomainRewrite
  onChange: (next: CloneDomainRewrite) => void
}) {
  const { t } = useTranslation('domains')
  return (
    <div className="space-y-2">
      <label className="flex items-start gap-2 text-sm">
        <Checkbox
          checked={value.copyDomains}
          onCheckedChange={(c) => {
            onChange({ ...value, copyDomains: c === true })
          }}
        />
        <span>
          {t('clone.copyDomains')}
          <span className="block text-xs text-muted-foreground">
            {t('clone.copyDomainsHint')}
          </span>
        </span>
      </label>
      {value.copyDomains ? (
        <div className="space-y-2 pl-6">
          <div className="flex gap-3 text-xs">
            {(['prefix', 'replace'] as const).map((mode) => (
              <label key={mode} className="flex items-center gap-1.5">
                <input
                  type="radio"
                  name="clone-domain-mode"
                  checked={value.mode === mode}
                  onChange={() => {
                    onChange({ ...value, mode })
                  }}
                />
                {t(`clone.mode.${mode}`)}
              </label>
            ))}
          </div>
          {value.mode === 'prefix' ? (
            <Field>
              <FieldLabel htmlFor="clone-domain-prefix">
                {t('clone.prefix')}
              </FieldLabel>
              <Input
                id="clone-domain-prefix"
                className="font-mono"
                value={value.prefix}
                placeholder={t('clone.prefixPlaceholder')}
                onChange={(e) => {
                  onChange({ ...value, prefix: e.target.value })
                }}
              />
            </Field>
          ) : (
            <div className="grid grid-cols-2 gap-2">
              <Field>
                <FieldLabel htmlFor="clone-domain-find">
                  {t('clone.find')}
                </FieldLabel>
                <Input
                  id="clone-domain-find"
                  className="font-mono"
                  value={value.find}
                  placeholder={t('clone.findPlaceholder')}
                  onChange={(e) => {
                    onChange({ ...value, find: e.target.value })
                  }}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="clone-domain-replace">
                  {t('clone.replace')}
                </FieldLabel>
                <Input
                  id="clone-domain-replace"
                  className="font-mono"
                  value={value.replace}
                  placeholder={t('clone.replacePlaceholder')}
                  onChange={(e) => {
                    onChange({ ...value, replace: e.target.value })
                  }}
                />
              </Field>
            </div>
          )}
        </div>
      ) : null}
    </div>
  )
}
