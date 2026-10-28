import { useTranslation } from 'react-i18next'
import { CopyIcon, EyeIcon, EyeSlashIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'

export interface TemplateCredentialRowProps {
  secretKey: string
  value?: string
  canReveal: boolean
  onReveal: () => void
  onHide: () => void
  onCopy: (value: string) => void
}

const MASK = '••••••••'

/** The value is never in the DOM until revealed; a mask stands in for it. */
export function TemplateCredentialRow({
  secretKey,
  value,
  canReveal,
  onReveal,
  onHide,
  onCopy,
}: TemplateCredentialRowProps) {
  const { t } = useTranslation('templates')
  return (
    <li className="flex min-w-0 flex-wrap max-sm:[&_button]:min-h-11 max-sm:[&_button]:min-w-11 items-center gap-2">
      <code className="min-w-0 break-words font-mono text-xs text-muted-foreground [overflow-wrap:anywhere]">
        {secretKey}
      </code>
      {value === undefined ? (
        <span aria-hidden="true" className="font-mono text-muted-foreground">
          {MASK}
        </span>
      ) : (
        <code
          className="min-w-0 flex-1 break-words rounded-md bg-muted px-2 py-0.5 font-mono text-sm [overflow-wrap:anywhere]"
          title={value}
        >
          {value}
        </code>
      )}
      <span className="ml-auto flex items-center gap-1">
        {value !== undefined ? (
          <>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={t('result.copy', { key: secretKey })}
              onClick={() => {
                onCopy(value)
              }}
            >
              <CopyIcon aria-hidden="true" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={t('result.hide', { key: secretKey })}
              onClick={onHide}
            >
              <EyeSlashIcon aria-hidden="true" />
            </Button>
          </>
        ) : canReveal ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t('result.reveal', { key: secretKey })}
            onClick={onReveal}
          >
            <EyeIcon aria-hidden="true" />
          </Button>
        ) : null}
      </span>
    </li>
  )
}
