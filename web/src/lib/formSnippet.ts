// Mirrors internal/forms/snippet.go so the live preview matches what the server hands out.

export type FormFieldType =
  | 'text'
  | 'email'
  | 'textarea'
  | 'select'
  | 'checkbox'
  | 'number'
  | 'url'
  | 'hidden'

export const FIELD_TYPES: FormFieldType[] = [
  'text',
  'email',
  'textarea',
  'select',
  'checkbox',
  'number',
  'url',
  'hidden',
]

export interface FormField {
  name: string
  type: FormFieldType
  label?: string
  required?: boolean
  placeholder?: string
  min_length?: number
  max_length?: number
  options?: string[]
  default?: string
}

export interface FormSchema {
  fields: FormField[]
  success_message?: string
  allowed_origins?: string[]
}

export function defaultFields(): FormField[] {
  return [
    { name: 'name', type: 'text', label: 'Name', required: true },
    { name: 'email', type: 'email', label: 'Email', required: true },
    { name: 'message', type: 'textarea', label: 'Message' },
  ]
}

export function effectiveFields(schema: FormSchema | undefined): FormField[] {
  return schema && schema.fields.length > 0 ? schema.fields : defaultFields()
}

export function escapeHtml(v: string): string {
  return v
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&#34;')
    .replace(/'/g, '&#39;')
}

function attrs(f: FormField, labels: boolean): string {
  let a = ''
  if (f.required) a += ' required'
  if (f.placeholder) a += ` placeholder="${escapeHtml(f.placeholder)}"`
  else if (!labels && f.type !== 'checkbox' && f.type !== 'select')
    a += ` placeholder="${escapeHtml(f.label || f.name)}"`
  if (f.type !== 'checkbox' && f.type !== 'select') {
    if (f.min_length) a += ` minlength="${f.min_length}"`
    if (f.max_length) a += ` maxlength="${f.max_length}"`
  }
  return a
}

function fieldHtml(f: FormField, labels: boolean): string {
  const name = escapeHtml(f.name)
  if (f.type === 'hidden')
    return `<input type="hidden" name="${name}" value="${escapeHtml(f.default ?? '')}">`
  let ctl: string
  if (f.type === 'textarea') {
    ctl = `<textarea name="${name}"${attrs(f, labels)}></textarea>`
  } else if (f.type === 'select') {
    const opts = (f.options ?? [])
      .map((o) => `<option value="${escapeHtml(o)}">${escapeHtml(o)}</option>`)
      .join('')
    ctl = `<select name="${name}"${attrs(f, labels)}>${opts}</select>`
  } else {
    ctl = `<input type="${f.type}" name="${name}"${attrs(f, labels)}>`
  }
  return labels ? `<label>${escapeHtml(f.label || f.name)} ${ctl}</label>` : ctl
}

export function buildHtml(
  endpoint: string,
  honeypot: string,
  schema: FormSchema | undefined,
  labels = true,
): string {
  const hp = escapeHtml(honeypot || 'bot-field')
  const lines = [
    `<form action="${escapeHtml(endpoint)}" method="POST">`,
    `  <p hidden><label>Leave empty <input name="${hp}" tabindex="-1" autocomplete="off"></label></p>`,
    ...effectiveFields(schema).map((f) => `  ${fieldHtml(f, labels)}`),
    '  <button type="submit">Send</button>',
    '</form>',
  ]
  return lines.join('\n')
}

function jsString(s: string): string {
  return JSON.stringify(s).replace(/</g, '\\u003c')
}

export function buildFetch(
  endpoint: string,
  honeypot: string,
  schema: FormSchema | undefined,
): string {
  const msg = schema?.success_message || 'Thanks, your submission was received.'
  const html = buildHtml(endpoint, honeypot, schema).replace(
    '<form ',
    '<form id="lr-form" ',
  )
  return `${html}
<p id="lr-status" role="status"></p>
<script>
document.getElementById("lr-form").addEventListener("submit", async function (e) {
  e.preventDefault();
  var data = Object.fromEntries(new FormData(e.target));
  var res = await fetch(${jsString(endpoint)}, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Accept": "application/json" },
    body: JSON.stringify(data)
  });
  document.getElementById("lr-status").textContent = res.ok ? ${jsString(msg)} : "Something went wrong, please try again.";
  if (res.ok) e.target.reset();
});
</script>`
}
