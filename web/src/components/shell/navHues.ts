// Maps nav group and item ids to a sidebar hue token. Class strings are
// written out in full so Tailwind can see them.
export type NavHue =
  'overview' | 'build' | 'data' | 'network' | 'fleet' | 'security' | 'ai'

export interface NavHueClasses {
  // Icon colour at rest, active icon, active bar and active tint.
  icon: string
  dot: string
  text: string
}

const GROUP_HUE: Record<string, NavHue> = {
  primary: 'overview',
  build: 'build',
  data: 'data',
  traffic: 'network',
  infrastructure: 'fleet',
  access: 'security',
  ai: 'ai',
}

const ITEM_HUE: Record<string, NavHue> = {
  dashboard: 'overview',
  apps: 'build',
  deployments: 'build',
  databases: 'data',
  domains: 'network',
  nodes: 'fleet',
  alerts: 'security',
  assistant: 'ai',
  console: 'ai',
}

const CLASSES: Record<NavHue, NavHueClasses> = {
  overview: {
    icon: '[&_svg]:text-sidebar-hue-overview data-active:before:bg-sidebar-hue-overview data-active:bg-sidebar-hue-overview/10 data-active:[&_svg]:text-sidebar-hue-overview',
    dot: 'bg-sidebar-hue-overview',
    text: 'text-sidebar-hue-overview',
  },
  build: {
    icon: '[&_svg]:text-sidebar-hue-build data-active:before:bg-sidebar-hue-build data-active:bg-sidebar-hue-build/10 data-active:[&_svg]:text-sidebar-hue-build',
    dot: 'bg-sidebar-hue-build',
    text: 'text-sidebar-hue-build',
  },
  data: {
    icon: '[&_svg]:text-sidebar-hue-data data-active:before:bg-sidebar-hue-data data-active:bg-sidebar-hue-data/10 data-active:[&_svg]:text-sidebar-hue-data',
    dot: 'bg-sidebar-hue-data',
    text: 'text-sidebar-hue-data',
  },
  network: {
    icon: '[&_svg]:text-sidebar-hue-network data-active:before:bg-sidebar-hue-network data-active:bg-sidebar-hue-network/10 data-active:[&_svg]:text-sidebar-hue-network',
    dot: 'bg-sidebar-hue-network',
    text: 'text-sidebar-hue-network',
  },
  fleet: {
    icon: '[&_svg]:text-sidebar-hue-fleet data-active:before:bg-sidebar-hue-fleet data-active:bg-sidebar-hue-fleet/10 data-active:[&_svg]:text-sidebar-hue-fleet',
    dot: 'bg-sidebar-hue-fleet',
    text: 'text-sidebar-hue-fleet',
  },
  security: {
    icon: '[&_svg]:text-sidebar-hue-security data-active:before:bg-sidebar-hue-security data-active:bg-sidebar-hue-security/10 data-active:[&_svg]:text-sidebar-hue-security',
    dot: 'bg-sidebar-hue-security',
    text: 'text-sidebar-hue-security',
  },
  ai: {
    icon: '[&_svg]:text-sidebar-hue-ai data-active:before:bg-sidebar-hue-ai data-active:bg-sidebar-hue-ai/10 data-active:[&_svg]:text-sidebar-hue-ai',
    dot: 'bg-sidebar-hue-ai',
    text: 'text-sidebar-hue-ai',
  },
}

export function hueForGroup(groupId: string | undefined): NavHue | undefined {
  return groupId ? GROUP_HUE[groupId] : undefined
}

export function hueForItem(
  itemId: string,
  groupId?: string,
): NavHue | undefined {
  return ITEM_HUE[itemId] ?? hueForGroup(groupId)
}

export function hueClasses(hue: NavHue | undefined): NavHueClasses | undefined {
  return hue ? CLASSES[hue] : undefined
}
