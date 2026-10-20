// The CLI binary is the brand binary name plus "-cli" (cmd/levelrail-cli).
export function cliCommand(binaryName: string, args: string): string {
  return `${binaryName ? `${binaryName}-cli` : 'cli'} ${args}`
}
