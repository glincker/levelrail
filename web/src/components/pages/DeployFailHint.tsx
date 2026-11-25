import { useTranslation } from 'react-i18next'
import { pagesFailHint } from '../../lib/pagesFailHint'

/** One plain-language fix hint for a failed build, picked from its log lines. Renders nothing when no known shape matches. */
export default function DeployFailHint({
  lines,
}: Readonly<{ lines: readonly string[] }>) {
  const { t } = useTranslation('pagesSites')
  const hint = pagesFailHint(lines)
  if (!hint) return null
  return (
    <p className="mt-1 font-sans text-foreground" role="note">
      {t(`logs.hint.${hint}`)}
    </p>
  )
}
