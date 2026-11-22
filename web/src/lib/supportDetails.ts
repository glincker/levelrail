import { browserInfo, routeTemplate } from './issueReport'

// Non-sensitive context for a support request: route, version, browser, time. No tokens or bodies.
export function supportDetails(
  header: string,
  pathname: string,
  version?: string,
): string {
  const b = browserInfo(navigator)
  return [
    header,
    `route: ${routeTemplate(pathname)}`,
    version ? `version: ${version}` : null,
    `browser: ${b.browser} on ${b.os}`,
    `time: ${new Date().toISOString()}`,
  ]
    .filter((l): l is string => l !== null)
    .join('\n')
}
