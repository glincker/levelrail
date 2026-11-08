import { translateShared } from './activeLocale'

// Pure load balancer helpers run outside React, so they read the shared
// translator; the English fallback keeps tests and first paint readable.
export function lbT(
  key: string,
  fallback: string,
  vars?: Record<string, string | number>,
): string {
  let out = translateShared(`statePlaceholders:lb.${key}`, fallback)
  for (const [k, v] of Object.entries(vars ?? {})) {
    out = out.replaceAll(`{{${k}}}`, String(v))
  }
  return out
}
