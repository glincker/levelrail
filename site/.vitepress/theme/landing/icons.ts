import {
  PhBell,
  PhChartLineUp,
  PhClockCounterClockwise,
  PhCube,
  PhCurrencyDollar,
  PhDatabase,
  PhEye,
  PhGauge,
  PhGitBranch,
  PhGitPullRequest,
  PhGithubLogo,
  PhGlobe,
  PhHardDrives,
  PhKey,
  PhLockKey,
  PhNetwork,
  PhPlugsConnected,
  PhRobot,
  PhScroll,
  PhTerminalWindow,
} from '@phosphor-icons/vue'

// One registry so a new icon is added here once and usable from any page's frontmatter.
export const landingIcons = {
  bell: PhBell,
  chart: PhChartLineUp,
  history: PhClockCounterClockwise,
  cube: PhCube,
  database: PhDatabase,
  dollar: PhCurrencyDollar,
  eye: PhEye,
  gauge: PhGauge,
  branch: PhGitBranch,
  pullrequest: PhGitPullRequest,
  github: PhGithubLogo,
  globe: PhGlobe,
  harddrives: PhHardDrives,
  key: PhKey,
  lockkey: PhLockKey,
  network: PhNetwork,
  plugs: PhPlugsConnected,
  robot: PhRobot,
  scroll: PhScroll,
  terminal: PhTerminalWindow,
} as const

export type LandingIconKey = keyof typeof landingIcons

export function landingIcon(key?: string) {
  return landingIcons[(key as LandingIconKey) in landingIcons ? (key as LandingIconKey) : 'cube']
}
