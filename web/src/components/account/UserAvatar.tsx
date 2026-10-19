import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { avatarInitials, avatarTone, gravatarUrl } from '../../lib/avatar'
import { cn } from '@/lib/utils'

const SIZES = {
  sm: 'size-8 text-xs',
  md: 'size-12 text-base',
  lg: 'size-16 text-xl',
} as const

export interface UserAvatarProps {
  name: string
  seed: string
  email?: string
  gravatar?: boolean
  size?: keyof typeof SIZES
  className?: string
}

// Initials on a color derived from the seed. A Gravatar is requested only
// when the user opted in, and any failure falls back to the initials.
export function UserAvatar({
  name,
  seed,
  email,
  gravatar = false,
  size = 'md',
  className,
}: UserAvatarProps) {
  const { t } = useTranslation('account')
  const [photo, setPhoto] = useState<{ email: string; url: string } | null>(
    null,
  )
  const wanted = gravatar && email !== undefined && email !== ''

  useEffect(() => {
    if (!wanted) return
    let cancelled = false
    gravatarUrl(email).then(
      (url) => {
        if (!cancelled) setPhoto({ email, url })
      },
      () => {
        if (!cancelled) setPhoto(null)
      },
    )
    return () => {
      cancelled = true
    }
  }, [wanted, email])

  const shown = wanted && photo?.email === email ? photo.url : null

  const label = t('avatar.alt', { name })
  return (
    <span
      role="img"
      aria-label={label}
      className={cn(
        'relative inline-flex shrink-0 items-center justify-center overflow-hidden rounded-full font-semibold select-none',
        SIZES[size],
        avatarTone(seed),
        className,
      )}
    >
      <span aria-hidden="true">{avatarInitials(name)}</span>
      {shown ? (
        <img
          src={shown}
          alt=""
          referrerPolicy="no-referrer"
          className="absolute inset-0 size-full object-cover"
          onError={() => setPhoto(null)}
        />
      ) : null}
    </span>
  )
}
