import {
  PhSquaresFour, PhRocketLaunch, PhHeartbeat, PhStack, PhFolder, PhDatabase, PhCloudArrowUp,
  PhHardDrives, PhShareNetwork, PhGlobe, PhScales, PhPackage, PhKey, PhActivity, PhScroll,
  PhTerminal, PhGitBranch, PhGear, PhWrench, PhEye, PhClockCounterClockwise, PhBell, PhGauge,
} from '@phosphor-icons/vue'
import type { Component } from 'vue'

export const mockIcons: Record<string, Component> = {
  dashboard: PhGauge,
  status: PhHeartbeat,
  apps: PhStack,
  deployments: PhRocketLaunch,
  projects: PhFolder,
  databases: PhDatabase,
  backups: PhCloudArrowUp,
  nodes: PhHardDrives,
  network: PhShareNetwork,
  domains: PhGlobe,
  loadbalancers: PhScales,
  registry: PhPackage,
  credentials: PhKey,
  overview: PhSquaresFour,
  deploys: PhClockCounterClockwise,
  source: PhGitBranch,
  config: PhWrench,
  traffic: PhShareNetwork,
  observe: PhEye,
  metrics: PhActivity,
  logs: PhScroll,
  alerts: PhBell,
  exec: PhTerminal,
  settings: PhGear,
}
