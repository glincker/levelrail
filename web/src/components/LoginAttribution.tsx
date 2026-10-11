import type { SVGProps } from 'react'
import { useTranslation } from 'react-i18next'

const STUDIO_URL = 'https://glincker.com'
const THEAUTH_URL = 'https://github.com/glincker/theauth'

// thesvg.org slug "glincker", mono variant, inlined so the page works offline.
function StudioMark(props: SVGProps<SVGSVGElement>) {
  return (
    <svg
      viewBox="0 0 375 375"
      fill="currentColor"
      aria-hidden="true"
      {...props}
    >
      <path
        fillRule="evenodd"
        d="M 53.1 187.5 C 89.2 151.4 125.3 115.4 161.4 79.2 L 161.4 26.1 C 107.6 79.9 53.8 133.7 0 187.5 C 53.8 241.3 107.6 295.1 161.4 348.9 L 161.4 295.8 C 125.3 259.7 89.2 223.6 53.1 187.5 Z M 268.8 187.5 L 242.3 160.9 C 224 179.2 205.8 197.4 187.5 215.7 L 159.3 187.5 C 186.4 160.4 213.5 133.3 240.6 106.2 C 267.7 133.3 294.8 160.4 321.9 187.5 C 286 223.4 250 259.4 214.1 295.4 L 187.5 268.8 C 214.6 241.7 241.7 214.6 268.8 187.5 Z M 187.5 268.8 L 187.5 375 C 250 312.5 312.5 250 375 187.5 C 312.5 125 250 62.5 187.5 0 L 187.5 106.2 C 160.4 133.3 133.3 160.4 106.2 187.5 C 133.7 215 160.4 241.6 187.5 268.8 Z"
      />
    </svg>
  )
}

// thesvg.org slug "theauth", mono variant.
function TheAuthMark(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 64 64" fill="none" aria-hidden="true" {...props}>
      <rect
        width="58"
        height="58"
        x="3"
        y="3"
        stroke="currentColor"
        strokeWidth="3"
        rx="17"
      />
      <rect
        width="37"
        height="37"
        x="13.5"
        y="13.5"
        stroke="currentColor"
        strokeOpacity=".7"
        strokeWidth="3"
        rx="10.845"
      />
      <rect
        width="16"
        height="16"
        x="24"
        y="24"
        stroke="currentColor"
        strokeOpacity=".5"
        strokeWidth="3"
        rx="4.69"
      />
      <rect width="8" height="8" x="28" y="28" fill="currentColor" rx="2.64" />
    </svg>
  )
}

const linkClass =
  'inline-flex items-center gap-1.5 rounded-sm transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring'

export function LoginAttribution() {
  const { t } = useTranslation('signIn')
  return (
    <div className="relative z-10 flex flex-col items-start gap-2 pt-4 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
      <a
        href={THEAUTH_URL}
        target="_blank"
        rel="noopener noreferrer"
        aria-label={t('attribution.theAuthLabel')}
        className={linkClass}
      >
        <TheAuthMark className="size-3.5" />
        <span>{t('attribution.poweredBy', { name: 'TheAuth' })}</span>
      </a>
      <a
        href={STUDIO_URL}
        target="_blank"
        rel="noopener noreferrer"
        aria-label={t('attribution.studioLabel')}
        className={linkClass}
      >
        <StudioMark className="size-3.5" />
        <span className="font-medium tracking-wide">GLINR STUDIOS</span>
      </a>
    </div>
  )
}
