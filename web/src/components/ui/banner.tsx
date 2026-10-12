import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  InfoIcon,
  MegaphoneSimpleIcon,
  WarningIcon,
  WarningOctagonIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Display, Eyebrow, Lede } from '@/components/ui/typography'
import { TONE, type Tone } from '@/components/kit/tone'
import { dismissBanner, isBannerDismissed } from '@/lib/bannerDismissal'
import { cn } from '@/lib/utils'

export type BannerVariant =
  'info' | 'success' | 'warning' | 'critical' | 'promo'

const VARIANT: Record<
  BannerVariant,
  { tone: Tone; icon: ReactNode; role: 'status' | 'alert' }
> = {
  info: { tone: 'info', icon: <InfoIcon />, role: 'status' },
  success: { tone: 'success', icon: <CheckCircleIcon />, role: 'status' },
  warning: { tone: 'warning', icon: <WarningIcon />, role: 'status' },
  critical: { tone: 'danger', icon: <WarningOctagonIcon />, role: 'alert' },
  promo: { tone: 'accent', icon: <MegaphoneSimpleIcon />, role: 'status' },
}

export interface BannerProps {
  variant?: BannerVariant
  title: ReactNode
  children?: ReactNode
  /** Small mono label above the title. */
  eyebrow?: ReactNode
  icon?: ReactNode
  /** Right-aligned button or link. */
  action?: ReactNode
  /** Stable id: when set the banner is dismissible and stays dismissed. */
  dismissId?: string
  onDismiss?: () => void
  className?: string
}

/** Banner is a page-level notice; dismissals persist in localStorage when `dismissId` is set. */
export function Banner({
  variant = 'info',
  title,
  children,
  eyebrow,
  icon,
  action,
  dismissId,
  onDismiss,
  className,
}: BannerProps) {
  const { t } = useTranslation('common')
  const [hidden, setHidden] = useState(() =>
    dismissId ? isBannerDismissed(dismissId) : false,
  )
  if (hidden) return null
  const v = VARIANT[variant]
  const tone = TONE[v.tone]
  const dismissible = Boolean(dismissId ?? onDismiss)

  return (
    <section
      role={v.role}
      data-variant={variant}
      className={cn(
        'flex items-start gap-3 rounded-lg border p-(--glinr-banner-pad)',
        tone.border,
        tone.soft,
        className,
      )}
    >
      <span
        aria-hidden="true"
        className={cn('mt-0.5 shrink-0 [&_svg]:size-5', tone.text)}
      >
        {icon ?? v.icon}
      </span>
      <div className="min-w-0 flex-1 space-y-0.5">
        {eyebrow ? <Eyebrow>{eyebrow}</Eyebrow> : null}
        <Display as="h3" size="banner">
          {title}
        </Display>
        {children ? (
          <Lede className="text-foreground/80">{children}</Lede>
        ) : null}
      </div>
      {action ? <div className="shrink-0 self-center">{action}</div> : null}
      {dismissible ? (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t('foundation.banner.dismiss')}
          onClick={() => {
            if (dismissId) dismissBanner(dismissId)
            setHidden(true)
            onDismiss?.()
          }}
        >
          <XIcon aria-hidden="true" />
        </Button>
      ) : null}
    </section>
  )
}
