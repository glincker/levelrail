import type { DropSelection } from '../../../lib/zipSite'
import { blankStarter } from './blank'
import { docsStarter } from './docs'
import { landingStarter } from './landing'
import { portfolioStarter } from './portfolio'
import type { Starter } from './shared'

export const STARTERS: readonly Starter[] = [
  blankStarter,
  landingStarter,
  docsStarter,
  portfolioStarter,
]

export type StarterId = Starter['id']

export function findStarter(id: StarterId): Starter {
  return STARTERS.find((s) => s.id === id) ?? blankStarter
}

/** Packs a starter into the same selection shape a folder drop produces, so publish stays one path. */
export function starterSelection(id: StarterId, name: string): DropSelection {
  const files = findStarter(id)
    .files(name)
    .map((f) => ({
      path: f.path,
      file: new File([f.content], f.path, { type: 'text/html' }),
    }))
  return {
    kind: 'folder',
    files,
    label: 'index.html',
    bytes: files.reduce((n, f) => n + f.file.size, 0),
    hasIndex: true,
  }
}

/** First file as a document for the preview iframe. */
export function starterPreviewHtml(id: StarterId, name: string): string {
  return findStarter(id).files(name)[0]?.content ?? ''
}
