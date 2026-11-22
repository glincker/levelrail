import { useTranslation } from 'react-i18next'
import { CodeBlock } from '@/components/ui/code-block'
import {
  deployButtonCardTags,
  deployButtonHtml,
  deployButtonMarkdown,
  templateDeployUrl,
} from '../../lib/deployLink'

/** Copyable README snippets (Markdown, HTML, social card tags) that deploy this template. */
export function TemplateDeployButton({
  id,
  name,
}: {
  id: string
  name: string
}) {
  const { t } = useTranslation('templates')
  const url = templateDeployUrl(id)
  if (url === null) return null
  const blocks = [
    ['markdown', deployButtonMarkdown(url), 'markdown'],
    ['html', deployButtonHtml(url), 'html'],
    ['card', deployButtonCardTags(url, name), 'html'],
  ] as const
  return (
    <section
      aria-labelledby="template-deploy-button"
      className="space-y-3 rounded-xl border border-border bg-card p-3 sm:p-4"
    >
      <div>
        <h2 id="template-deploy-button" className="text-base font-semibold">
          {t('deployButton.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('deployButton.hint')}
        </p>
      </div>
      {blocks.map(([key, code, language]) => (
        <div key={key} className="space-y-1">
          <h3 className="text-xs font-medium text-muted-foreground">
            {t(`deployButton.${key}`)}
          </h3>
          <CodeBlock
            code={code}
            language={language}
            label={t(`deployButton.${key}Label`)}
            maxHeight="lg"
          />
        </div>
      ))}
    </section>
  )
}
