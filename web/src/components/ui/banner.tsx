import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  InfoGlassIcon,
  MegaphoneGlassIcon,
  OctagonAlertGlassIcon,
  TriangleAlertGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Display, Eyebrow } from '@/components/ui/typography'
import { TONE, type Tone } from '@/components/kit/tone'
import { dismissBanner, isBannerDismissed } from '@/lib/bannerDismissal'
import { cn } from '@/lib/utils'

export type BannerVariant =
  'info' | 'success' | 'warning' | 'critical' | 'promo'

const VARIANT: Record<
  BannerVariant,
  { tone: Tone; icon: ReactNode; role: 'status' | 'alert' }
> = {
  info: {
    tone: 'info',
    icon: <InfoGlassIcon className="text-tone-info" size="1.2em" />,
    role: 'status',
  },
  success: {
    tone: 'success',
    icon: <CircleCheckGlassIcon className="text-tone-success" size="1.2em" />,
    role: 'status',
  },
  warning: {
    tone: 'warning',
    icon: <TriangleAlertGlassIcon className="text-tone-warning" size="1.2em" />,
    role: 'status',
  },
  critical: {
    tone: 'danger',
    icon: <OctagonAlertGlassIcon className="text-tone-danger" size="1.2em" />,
    role: 'alert',
  },
  promo: {
    tone: 'accent',
    icon: <MegaphoneGlassIcon size="1.2em" />,
    role: 'status',
  },
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
        className={cn(
          'flex h-5 shrink-0 items-center [&_svg]:size-5',
          tone.text,
        )}
      >
        {icon ?? v.icon}
      </span>
      <div className="min-w-0 flex-1 space-y-0.5">
        {eyebrow ? <Eyebrow>{eyebrow}</Eyebrow> : null}
        {children ? (
          <Display as="h3" size="banner">
            {title}
          </Display>
        ) : (
          <p className="text-sm leading-5 font-normal text-foreground">
            {title}
          </p>
        )}
        {children ? (
          <div className="max-w-prose text-sm text-pretty text-foreground/80">
            {children}
          </div>
        ) : null}
      </div>
      {action ? <div className="shrink-0 self-center">{action}</div> : null}
      {dismissible ? (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="-my-1 shrink-0"
          aria-label={t('foundation.banner.dismiss')}
          onClick={() => {
            if (dismissId) dismissBanner(dismissId)
            setHidden(true)
            onDismiss?.()
          }}
        >
          <XGlassIcon size="1.2em" aria-hidden="true" />
        </Button>
      ) : null}
    </section>
  )
}
