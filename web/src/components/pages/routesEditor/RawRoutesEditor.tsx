import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Textarea } from '@/components/ui/textarea'
import type { DraftRedirect } from '../rulesDraft'
import { redirectsToText, textToRedirects } from './rawText'

/** Text view of the redirect rows. Text is local while typing so partial lines are not rewritten. */
export function RawRoutesEditor({
  rows,
  onChange,
}: Readonly<{
  rows: readonly DraftRedirect[]
  onChange: (rows: DraftRedirect[]) => void
}>) {
  const { t } = useTranslation('pagesSites')
  const [text, setText] = useState(() => redirectsToText(rows))
  return (
    <div className="space-y-1">
      <Textarea
        aria-label={t('routes.raw.label')}
        value={text}
        rows={Math.max(4, text.split('\n').length + 1)}
        spellCheck={false}
        className="font-mono text-xs"
        placeholder={t('routes.raw.placeholder')}
        onChange={(e) => {
          setText(e.target.value)
          onChange(textToRedirects(e.target.value, rows))
        }}
      />
      <p className="text-xs text-muted-foreground">{t('routes.raw.hint')}</p>
    </div>
  )
}
