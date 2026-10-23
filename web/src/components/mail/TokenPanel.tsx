import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { useMailTokens, type MailToken } from '../../queries/mail'

const SCOPES: MailToken['scope'][] = ['global', 'recipient', 'event']

/** TokenPanel lists the tokens an email supports; clicking one hands its placeholder to onInsert. */
export function TokenPanel({
  allowed,
  onInsert,
}: {
  allowed: string[]
  onInsert: (placeholder: string) => void
}) {
  const { t } = useTranslation('mail')
  const { data: tokens = [] } = useMailTokens()
  const usable = tokens.filter((tok) => allowed.includes(tok.name))

  return (
    <section aria-label={t('templates.tokens.title')} className="space-y-3">
      <div>
        <h3 className="text-sm font-medium">{t('templates.tokens.title')}</h3>
        <p className="text-xs text-muted-foreground">
          {t('templates.tokens.description')}
        </p>
      </div>
      {SCOPES.map((scope) => {
        const group = usable.filter((tok) => tok.scope === scope)
        if (group.length === 0) return null
        return (
          <div key={scope} className="space-y-1.5">
            <p className="text-xs font-medium text-muted-foreground">
              {t(`templates.tokens.scope.${scope}`)}
            </p>
            <ul className="flex flex-wrap gap-1.5">
              {group.map((tok) => (
                <li key={tok.name}>
                  <button
                    type="button"
                    title={`${tok.description} (${tok.sample})`}
                    aria-label={t('templates.tokens.insert', {
                      name: tok.name,
                    })}
                    onClick={() => onInsert(`{{${tok.name}}}`)}
                    className="rounded-md focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                  >
                    <Badge variant="outline" className="font-mono">
                      {tok.name}
                    </Badge>
                  </button>
                </li>
              ))}
            </ul>
          </div>
        )
      })}
    </section>
  )
}
