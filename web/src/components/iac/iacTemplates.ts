// Multi-file templates become one multi-document YAML so the editor stays a single buffer.
export function joinTemplateFiles(files: { content: string }[]): string {
  return files.map((f) => f.content.trimEnd()).join('\n---\n') + '\n'
}
