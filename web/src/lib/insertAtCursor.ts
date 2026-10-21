/** insertAtCursor splices text into value at the element's selection and returns the new value and caret. */
export function insertAtCursor(
  el: HTMLInputElement | HTMLTextAreaElement | null,
  value: string,
  text: string,
): { value: string; caret: number } {
  const start = el?.selectionStart ?? value.length
  const end = el?.selectionEnd ?? value.length
  return {
    value: value.slice(0, start) + text + value.slice(end),
    caret: start + text.length,
  }
}
