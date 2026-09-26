export function parseRecipients(text: string): string[] {
  return text
    .split(/[\s,]+/)
    .map((r) => r.trim())
    .filter((r) => r !== '')
}

export function recipientProblem(recipients: string[]): string | null {
  for (const r of recipients) {
    if (r.startsWith('AGE-SECRET-KEY')) {
      return 'That is a private key. Paste the public key (it starts with age1) and keep the private one offline.'
    }
    if (!r.startsWith('age1')) {
      return `"${r.slice(0, 12)}" is not an age public key (they start with age1).`
    }
  }
  return null
}
