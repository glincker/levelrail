// Post-paint styles (route transitions, scrolled bar material, large-title collapse) stay out of the first-load CSS.
let loading: Promise<unknown> | null = null

export function loadDeferredStyles(): void {
  loading ??= import('@/styles/deferred.css')
}
