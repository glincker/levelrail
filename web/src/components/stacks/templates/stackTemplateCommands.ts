import { cliCommand } from '../../../lib/cliCommand'
import type { StackTemplateFile } from '../../../types/stackTemplates'

const SAFE = /^[A-Za-z0-9_@%+=:,./-]+$/

function quote(value: string): string {
  return SAFE.test(value) ? value : `'${value.replaceAll("'", "'\\''")}'`
}

/** The init then apply commands for one template; apply stays a CLI step in this view. */
export function stackTemplateCommands(
  binaryName: string,
  id: string,
  variables: Record<string, string>,
  secretNames: readonly string[],
): string {
  const stackName = variables.name || id
  const sets = Object.entries(variables)
    .map(([k, v]) => `--set ${k}=${quote(v)}`)
    .join(' ')
  const secrets = secretNames.map((n) => ` --secret ${n}=${n}`).join('')
  return [
    cliCommand(binaryName, `stack init ${id}${sets ? ` ${sets}` : ''}`),
    `cd ${quote(stackName)}`,
    cliCommand(binaryName, `stack apply ${quote(stackName)} -f .${secrets}`),
  ].join('\n')
}

export function joinStackTemplateFiles(
  files: readonly StackTemplateFile[],
): string {
  return files.map((f) => `# ${f.name}\n${f.content}`).join('\n')
}

export function downloadStackTemplateFiles(
  files: readonly StackTemplateFile[],
): void {
  for (const f of files) {
    const url = URL.createObjectURL(
      new Blob([f.content], { type: 'text/plain' }),
    )
    const a = document.createElement('a')
    a.href = url
    a.download = f.name
    a.click()
    URL.revokeObjectURL(url)
  }
}
