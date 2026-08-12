// Build logs commonly carry ANSI escape codes (colored compiler output,
// progress spinners, cursor movement). Full ANSI-to-HTML rendering
// (mapping SGR color codes to styled spans) is a stretch goal, not
// required for this pass: see the deploy logs route's comment. This
// strips the escape sequences so they do not render as garbage control
// characters, without attempting to preserve or translate the color or
// style information they carried.
//
// Pattern is the well-established one from the `ansi-regex` npm package
// (sindresorhus/ansi-regex, MIT), inlined here rather than pulled in as a
// dependency since this is the only place ANSI handling happens and a
// route this deliberately isolated (frontend-plan.md section 1) should
// not grow its dependency surface for a five-line regex. Matches both CSI
// sequences (ESC [ ... <final byte>, e.g. "\x1B[31m" for color, "\x1B[2K"
// to clear a line) and OSC sequences (ESC ] ... BEL, e.g. hyperlink/title
// codes some build tools emit).
const ANSI_PATTERN = new RegExp(
  [
    '[\\u001B\\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\\u0007)',
    '(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PR-TZcf-ntqry=><~]))',
  ].join('|'),
  'g',
)

export function stripAnsiCodes(input: string): string {
  return input.replace(ANSI_PATTERN, '')
}
