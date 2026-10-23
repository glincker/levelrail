import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { EyeIcon, EyeSlashIcon } from '@phosphor-icons/react/dist/ssr'

import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'

type SecretInputProps = Omit<React.ComponentProps<'input'>, 'type'> & {
  // Shows a masked placeholder while empty so a stored secret reads as set.
  hasExistingValue?: boolean
}

function SecretInput({
  className,
  hasExistingValue = false,
  autoComplete = 'new-password',
  placeholder,
  disabled,
  onBlur,
  ...props
}: SecretInputProps) {
  const { t } = useTranslation('common')
  const [revealed, setRevealed] = React.useState(false)
  const label = revealed ? t('secretInput.hide') : t('secretInput.show')

  // Hide again once focus leaves the whole control, not just the input.
  const handleBlur = (e: React.FocusEvent<HTMLDivElement>) => {
    if (!e.currentTarget.contains(e.relatedTarget)) {
      setRevealed(false)
    }
  }

  return (
    <div
      data-slot="secret-input"
      className="relative w-full"
      onBlur={handleBlur}
    >
      <Input
        {...props}
        type={revealed ? 'text' : 'password'}
        autoComplete={autoComplete}
        disabled={disabled}
        placeholder={hasExistingValue ? t('secretInput.existing') : placeholder}
        onBlur={onBlur}
        className={cn('pr-9', className)}
      />
      <button
        type="button"
        aria-pressed={revealed}
        aria-label={label}
        title={label}
        disabled={disabled}
        // Keep focus on the input so the click is not seen as a blur-out.
        onMouseDown={(e) => {
          e.preventDefault()
        }}
        onClick={() => {
          setRevealed((v) => !v)
        }}
        className="absolute inset-y-0 right-0 flex w-8 items-center justify-center rounded-r-lg text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50"
      >
        {revealed ? (
          <EyeSlashIcon aria-hidden className="size-4" />
        ) : (
          <EyeIcon aria-hidden className="size-4" />
        )}
      </button>
    </div>
  )
}

export { SecretInput }
export type { SecretInputProps }
