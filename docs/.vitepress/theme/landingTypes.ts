export interface LandingLine {
  k: 'cmd' | 'out' | 'ok'
  t: string
}

export interface LandingCard {
  title: string
  body: string
  icon?: string
  visual?: LandingLine[]
  link?: { text: string; href: string }
}

export interface LandingData {
  eyebrow?: string
  headline: string
  sub: string
  primary: { text: string; link: string }
  secondary?: { text: string; link: string }
  install?: string
  shot?: { src: string; alt: string }
  stats?: { value: string; label: string }[]
  cardsHeading?: string
  cards?: LandingCard[]
  terminal?: {
    heading?: string
    intro?: string
    title?: string
    ariaLabel?: string
    lines: { kind: 'command' | 'output' | 'success'; text: string }[]
  }
  compare?: {
    heading: string
    intro?: string
    left: string
    right: string
    rows: { label: string; left: string; right: string }[]
    more?: { text: string; href: string }
  }
  stepsHeading?: string
  steps?: { title: string; body: string }[]
  galleryHeading?: string
  gallery?: { src: string; alt: string; caption: string }[]
  prose?: { heading: string; paragraphs: string[] }[]
  faq?: { q: string; a: string }[]
  cta: { heading: string; sub: string }
  related?: { text: string; link: string }[]
}
