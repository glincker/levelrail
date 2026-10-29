import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CircleGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import {
  MIN_PASSWORD_LENGTH,
  passwordRequirements,
  scorePassword,
  type PasswordScore,
} from '../../lib/passwordStrength'

const BAR_TONE: Record<PasswordScore, string> = {
  0: 'bg-muted',
  1: 'bg-destructive',
  2: 'bg-amber-500',
  3: 'bg-emerald-500',
  4: 'bg-emerald-600',
}

const RULES = ['length', 'mixedCase', 'digit', 'symbol'] as const

export function PasswordStrengthMeter({ password }: { password: string }) {
  const { t } = useTranslation('settings')
  const score = scorePassword(password)
  const req = passwordRequirements(password)
  return (
    <div className="space-y-2" data-testid="password-strength">
      <div className="flex items-center gap-2">
        <div
          className="flex flex-1 gap-1"
          role="meter"
          aria-label={t('accountPage.password.strength.label')}
          aria-valuemin={0}
          aria-valuemax={4}
          aria-valuenow={score}
          aria-valuetext={t(`accountPage.password.strength.level${score}`)}
        >
          {[1, 2, 3, 4].map((n) => (
            <span
              key={n}
              className={cn(
                'h-1.5 flex-1 rounded-full',
                n <= score ? BAR_TONE[score] : 'bg-muted',
              )}
            />
          ))}
        </div>
        <span className="w-16 text-right text-xs text-muted-foreground">
          {t(`accountPage.password.strength.level${score}`)}
        </span>
      </div>
      <ul className="grid gap-x-4 gap-y-1 text-xs sm:grid-cols-2">
        {RULES.map((rule) => (
          <li
            key={rule}
            className={cn(
              'flex items-center gap-1.5',
              req[rule] ? 'text-foreground' : 'text-muted-foreground',
            )}
          >
            {req[rule] ? (
              <CheckGlassIcon
                className="size-4 text-emerald-600"
                aria-hidden="true"
              />
            ) : (
              <CircleGlassIcon className="size-4" aria-hidden="true" />
            )}
            {t(`accountPage.password.rules.${rule}`, {
              min: MIN_PASSWORD_LENGTH,
            })}
            <span className="sr-only">
              {req[rule]
                ? t('accountPage.password.rules.met')
                : t('accountPage.password.rules.unmet')}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
