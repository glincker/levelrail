import { useState } from 'react'
import { CodeBlock } from '@/components/ui/code-block'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { DASHBOARD_URL_ANCHOR } from '../DashboardUrlCard'
import type { AppForm } from '../../queries/forms'
import { buildFetch, buildHtml, type FormSchema } from '../../lib/formSnippet'

type Variant = 'html' | 'fetch' | 'minimal'
const VARIANTS: Variant[] = ['html', 'fetch', 'minimal']

export function CopyBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="space-y-1">
      <span className="text-sm font-medium">{label}</span>
      <CodeBlock code={value} label={label} />
    </div>
  )
}

// draft carries the unsaved designer state so the snippet follows edits as they are typed.
export function FormEmbed({
  form,
  draft,
}: {
  form: AppForm
  draft: FormSchema | undefined
}) {
  const { t } = useTranslation('forms')
  const [variant, setVariant] = useState<Variant>('html')
  const schema = draft ?? form.schema
  const hp = form.honeypot_field
  const snippets: Record<Variant, string> = {
    html: buildHtml(form.endpoint, hp, schema),
    fetch: buildFetch(form.endpoint, hp, schema),
    minimal: buildHtml(form.endpoint, hp, schema, false),
  }
  return (
    <div className="space-y-3">
      {form.endpoint_source === 'local' && (
        <Alert>
          <WarningIcon />
          <AlertTitle>{t('embed.noPublicUrl.title')}</AlertTitle>
          <AlertDescription>
            {t('embed.noPublicUrl.body')}
            <div className="mt-2">
              <Link
                to="/domains"
                hash={DASHBOARD_URL_ANCHOR}
                className={cn(buttonVariants({ size: 'sm' }))}
              >
                {t('embed.noPublicUrl.fix')}
              </Link>
            </div>
          </AlertDescription>
        </Alert>
      )}
      <CopyBlock label={t('endpoint')} value={form.endpoint} />
      <div role="tablist" className="flex gap-1">
        {VARIANTS.map((v) => (
          <Button
            key={v}
            role="tab"
            aria-selected={variant === v}
            size="xs"
            variant={variant === v ? 'secondary' : 'ghost'}
            onClick={() => setVariant(v)}
          >
            {t(`embed.variants.${v}`)}
          </Button>
        ))}
      </div>
      <CopyBlock
        label={t(`embed.variantHint.${variant}`)}
        value={snippets[variant]}
      />
    </div>
  )
}
