import type { HeadConfig, PageData } from 'vitepress'
import raw from './data/templates.json'

export interface GalleryService {
  name: string
  image: string
  ports: number[]
  volumes: string[]
}
export interface GalleryEnv {
  service: string
  key: string
  kind: 'generated' | 'required' | 'preset'
}
export interface GalleryEntry {
  id: string
  name: string
  slogan: string
  category: string
  docUrl?: string
  memoryMiB?: number
  gpu?: boolean
  needsConfig: boolean
  services: GalleryService[]
  env: GalleryEnv[]
}

export const entries = raw as GalleryEntry[]

const MAX_DESCRIPTION = 158

function pageTitle(e: GalleryEntry): string {
  return `Self-host ${e.name} with Docker`
}

function pageDescription(e: GalleryEntry): string {
  const slogan = e.slogan.replace(/\.$/, '')
  const full = `${slogan}. Self-host ${e.name} with Docker on your own server using the Levelrail template.`
  if (full.length <= MAX_DESCRIPTION) return full
  return `Self-host ${e.name} with Docker on your own server using the Levelrail template: services, ports, volumes and env vars.`
}

export function galleryParams(e: GalleryEntry) {
  const related = entries
    .filter((o) => o.category === e.category && o.id !== e.id)
    .slice(0, 6)
    .map((o) => ({ id: o.id, name: o.name }))
  return { ...e, title: pageTitle(e), description: pageDescription(e), related }
}

function findEntry(pageData: PageData): GalleryEntry | undefined {
  const id = pageData.params?.id as string | undefined
  if (!id || !pageData.relativePath.startsWith('self-host/')) return undefined
  return entries.find((x) => x.id === id)
}

// Dynamic route pages share one markdown file, so title and description are filled in per template.
export function galleryPageData(pageData: PageData) {
  const e = findEntry(pageData)
  if (!e) return
  pageData.title = pageTitle(e)
  pageData.titleTemplate = ':title | Levelrail'
  pageData.description = pageDescription(e)
  pageData.frontmatter.title = pageTitle(e)
  pageData.frontmatter.description = pageDescription(e)
}

export function galleryHead(pageData: PageData, siteUrl: string): HeadConfig[] {
  const e = findEntry(pageData)
  if (!e) return []
  const ld = {
    '@context': 'https://schema.org',
    '@type': 'SoftwareApplication',
    name: e.name,
    description: e.slogan,
    applicationCategory: e.category,
    operatingSystem: 'Linux (Docker)',
    url: `${siteUrl}/self-host/${e.id}`,
    ...(e.docUrl ? { sameAs: e.docUrl } : {}),
  }
  return [['script', { type: 'application/ld+json' }, JSON.stringify(ld)]]
}
