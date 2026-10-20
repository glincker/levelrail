const KEY = 'demo.tour.dismissed.v1'

export function isTourDismissed(): boolean {
  try {
    return window.localStorage.getItem(KEY) === '1'
  } catch {
    return false
  }
}

export function dismissTour(): void {
  try {
    window.localStorage.setItem(KEY, '1')
  } catch {
    // Storage blocked: the tour stays closed for this view only.
  }
}

export interface TourStep {
  id: 'apps' | 'deploys' | 'metrics' | 'nodes'
  to: '/apps' | '/deployments' | '/nodes'
}

export const TOUR_STEPS: readonly [TourStep, ...TourStep[]] = [
  { id: 'apps', to: '/apps' },
  { id: 'deploys', to: '/deployments' },
  { id: 'metrics', to: '/apps' },
  { id: 'nodes', to: '/nodes' },
]
