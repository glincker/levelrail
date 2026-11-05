import { useState } from 'react'
import { cn } from '@/lib/utils'
import { avatarToneClass, orgInitials } from './orgSlug'

const SIZES = {
  xs: 'size-5 rounded text-xs',
  sm: 'size-7 rounded-md text-xs',
  lg: 'size-16 rounded-xl text-xl',
} as const

export interface OrgAvatarProps {
  id: string
  name: string
  avatarUrl?: string
  size?: keyof typeof SIZES
  className?: string
}

// Rounded square with the uploaded image, or initials on a tone derived from
// the org id when there is none or it fails to load.
export function OrgAvatar({
  id,
  name,
  avatarUrl,
  size = 'sm',
  className,
}: OrgAvatarProps) {
  const [failedUrl, setFailedUrl] = useState('')
  const showImage = Boolean(avatarUrl) && failedUrl !== avatarUrl
  return (
    <span
      data-testid="org-avatar"
      className={cn(
        'inline-flex shrink-0 items-center justify-center overflow-hidden font-semibold select-none',
        SIZES[size],
        !showImage && avatarToneClass(id),
        className,
      )}
    >
      {showImage ? (
        <img
          src={avatarUrl}
          alt=""
          className="size-full object-cover"
          onError={() => setFailedUrl(avatarUrl ?? '')}
        />
      ) : (
        <span aria-hidden="true">{orgInitials(name)}</span>
      )}
    </span>
  )
}
