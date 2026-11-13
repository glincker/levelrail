/** Text the user has selected inside `container`, or '' when nothing is selected there. */
export function readLogSelection(container: HTMLElement | null): string {
  if (!container) return ''
  const sel = window.getSelection()
  if (!sel || sel.isCollapsed || sel.rangeCount === 0) return ''
  const { anchorNode, focusNode } = sel
  if (
    !anchorNode ||
    !focusNode ||
    !container.contains(anchorNode) ||
    !container.contains(focusNode)
  ) {
    return ''
  }
  return sel.toString()
}
