// stripNoteComments removes HTML comments (release-notes markers) so they
// never reach the renderer, even as text.
export function stripNoteComments(markdown: string): string {
  return markdown.replace(/<!--[\s\S]*?(-->|$)/g, '').trim()
}
