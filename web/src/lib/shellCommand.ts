const COMMAND_START =
  /^(sudo |levelrail|[A-Z][A-Z0-9_]*=|\.\/|docker |systemctl |curl |sh |apt|dnf |yum |ufw |firewall-cmd |iptables |nft |chmod |chown |export )/

/** isShellCommand tells a runnable command apart from prose advice in a check's fix text. */
export function isShellCommand(text: string): boolean {
  return COMMAND_START.test(text.trim())
}
