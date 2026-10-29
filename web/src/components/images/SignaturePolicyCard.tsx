import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BadgeCheckGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useValidateSignaturePolicy } from '../../queries/imageInventory'

export function SignaturePolicyCard({ mode }: { mode: 'off' | 'enforce' }) {
  const { t } = useTranslation('images')
  const [ref, setRef] = useState('')
  const validate = useValidateSignaturePolicy()
  const res = validate.data

  return (
    <section
      className="space-y-3 rounded-lg border p-4"
      aria-label={t('policy.title')}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="space-y-0.5">
          <h2 className="flex items-center gap-2 text-sm font-medium">
            <BadgeCheckGlassIcon
              className="size-4.5 text-tone-success"
              aria-hidden="true"
            />
            {t('policy.title')}
          </h2>
          <p className="text-xs text-muted-foreground">
            {t(`signatures.${mode}`)}
          </p>
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={validate.isPending}
          onClick={() => {
            validate.mutate(ref.trim())
          }}
        >
          {validate.isPending ? t('policy.validating') : t('policy.validate')}
        </Button>
      </div>
      <Input
        value={ref}
        onChange={(e) => {
          setRef(e.target.value)
        }}
        placeholder={t('policy.refPlaceholder')}
        aria-label={t('policy.refLabel')}
      />
      {validate.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{t('policy.error')}</AlertDescription>
        </Alert>
      ) : null}
      {res ? (
        <div className="space-y-2 text-sm" role="status">
          <p>
            {res.ok ? t('policy.valid') : t('policy.invalid')}
            {res.key_fingerprint
              ? ` ${t('policy.fingerprint', { value: res.key_fingerprint })}`
              : ''}
          </p>
          {res.sample ? (
            <p className="text-muted-foreground">
              {t(`policy.sample.${res.sample.status}`)}
            </p>
          ) : null}
          {res.issues.map((is) => (
            <Alert key={is.code} variant="destructive">
              <AlertDescription>
                {t(`policy.issues.${is.code}`)}
              </AlertDescription>
            </Alert>
          ))}
        </div>
      ) : null}
    </section>
  )
}
