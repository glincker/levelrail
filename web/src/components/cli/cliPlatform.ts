// Windows has its own PowerShell installer (install-cli.ps1); macOS and
// Linux use the shell script. Anything else keeps the generic shell command.
export const CLI_INSTALL_COMMAND_WINDOWS =
  'irm https://levelrail.com/install-cli.ps1 | iex'

export type CliPlatform = 'windows' | 'unix'

interface NavigatorWithHints {
  userAgentData?: { platform?: string }
}

export function detectCliPlatform(): CliPlatform {
  if (typeof navigator === 'undefined') {
    return 'unix'
  }
  const hinted = (navigator as NavigatorWithHints).userAgentData?.platform
  const raw = hinted || navigator.platform || ''
  return /win/i.test(raw) && !/darwin/i.test(raw) ? 'windows' : 'unix'
}
