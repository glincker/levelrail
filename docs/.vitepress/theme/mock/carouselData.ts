import { PhArrowCounterClockwise, PhChartLineUp, PhSquaresFour, PhTerminalWindow } from '@phosphor-icons/vue'
import type { Component } from 'vue'

export type MockView = 'apps' | 'overview' | 'deploys' | 'logs'

export interface CarouselSlide {
  view: MockView
  label: string
  icon: Component
  caption: string
  link: { text: string; href: string }
}

export const showcaseSlides: CarouselSlide[] = [
  {
    view: 'apps',
    label: 'Apps',
    icon: PhSquaresFour,
    caption: 'Every app at a glance, with its health and the node it is placed on.',
    link: { text: 'How nodes and placement work', href: '/multi-node' },
  },
  {
    view: 'deploys',
    label: 'Deploy and rollback',
    icon: PhArrowCounterClockwise,
    caption: 'Deploy history with one-click rollback. Cleanup never touches a rollback target.',
    link: { text: 'Deploy safety', href: '/deploy-safety' },
  },
  {
    view: 'overview',
    label: 'Metrics',
    icon: PhChartLineUp,
    caption: 'Per-app metrics with deploy markers on the charts. They stay on each node, with no separate stack to install.',
    link: { text: 'Observability', href: '/observability' },
  },
  {
    view: 'logs',
    label: 'Live logs',
    icon: PhTerminalWindow,
    caption: 'Live log tail with full-text search, read from the node that runs the app.',
    link: { text: 'Observability', href: '/observability' },
  },
]

export const heroViews: MockView[] = ['overview', 'deploys', 'logs']
