import type { DocsManifest } from '../types/docs'
import { filePathToRoutePath, splitHash } from './docsPaths'

export interface ResolvedDocLink {
  href: string
  /** true when the link should open in a new tab (external, or an unresolved fallback to the hosted docs site). */
  external: boolean
  /** false only for an unresolved internal link with nowhere safe to point: render as plain text, never a dead link. */
  linkable: boolean
}

function isAbsoluteUrl(target: string): boolean {
  return /^([a-z][a-z0-9+.-]*:|\/\/)/i.test(target)
}

// Resolves a relative markdown segment ("../adr/x.md", "./y.md", "z.md")
// against the directory of the doc that contains the link, collapsing
// "." and ".." the same way a filesystem path join would.
function resolveRelative(currentFile: string, target: string): string {
  const currentDir = currentFile.includes('/')
    ? currentFile.slice(0, currentFile.lastIndexOf('/'))
    : ''
  const segments = currentDir ? currentDir.split('/') : []
  for (const part of target.split('/')) {
    if (part === '' || part === '.') continue
    if (part === '..') segments.pop()
    else segments.push(part)
  }
  return segments.join('/')
}

// Resolves a markdown link's raw href to either an in-app /help route, an
// external fallback to the hosted docs site, a left-alone external URL,
// or "not linkable" when none of those are safe (avoids ever emitting a
// link with nothing real behind it, offline or not).
export function resolveDocLink(
  href: string,
  currentFile: string,
  manifest: DocsManifest,
  docsBaseUrl: string,
): ResolvedDocLink {
  if (href.startsWith('#')) {
    return { href, external: false, linkable: true }
  }
  if (isAbsoluteUrl(href) || href.startsWith('mailto:')) {
    return { href, external: true, linkable: true }
  }

  const [targetPath, hash] = splitHash(href)

  // Anything with a non-".md" extension (an image, a CNAME file) is an
  // asset link, not a doc link: leave it alone rather than guess.
  const hasOtherExtension =
    /\.[a-z0-9]+$/i.test(targetPath) && !targetPath.endsWith('.md')
  if (hasOtherExtension) {
    return { href, external: false, linkable: true }
  }

  let resolvedFile: string
  if (targetPath.startsWith('/')) {
    resolvedFile = `${targetPath.replace(/^\//, '').replace(/\.md$/, '')}.md`
  } else if (targetPath.endsWith('.md')) {
    resolvedFile = resolveRelative(currentFile, targetPath)
  } else {
    // Extensionless relative link with no doc convention behind it
    // (a clean-URL-style link this docs set doesn't actually use):
    // leave it alone rather than guess.
    return { href, external: false, linkable: true }
  }

  const routePath = filePathToRoutePath(resolvedFile)
  if (manifest.pages[routePath]) {
    return { href: `/help${routePath}${hash}`, external: false, linkable: true }
  }
  if (docsBaseUrl) {
    return {
      href: `${docsBaseUrl}${routePath}${hash}`,
      external: true,
      linkable: true,
    }
  }
  return { href: '', external: false, linkable: false }
}
