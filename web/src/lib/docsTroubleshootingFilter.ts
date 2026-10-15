import type { DocsManifest } from '@/types/docs'

export function getTroubleshootingPages(manifest: DocsManifest): Set<string> {
  const troubleshootingPages = new Set<string>()

  for (const [path, page] of Object.entries(manifest.pages)) {
    const hasTroubleshootingHeading = page.headings.some(
      (h) => h.text.toLowerCase() === 'troubleshooting' && h.level === 2,
    )
    if (hasTroubleshootingHeading) {
      troubleshootingPages.add(path)
    }
  }

  return troubleshootingPages
}
