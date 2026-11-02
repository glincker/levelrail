// Hooks shared by consumers that do and do not show the error themselves take
// this option, so only the consumers that render it suppress the default toast.
export interface InlineErrorOptions {
  inlineError?: boolean
}

export function inlineErrorMeta(
  opts?: InlineErrorOptions,
): { errorToast: false } | undefined {
  return opts?.inlineError ? { errorToast: false } : undefined
}
