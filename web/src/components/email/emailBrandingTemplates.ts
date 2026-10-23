import type { EmailBranding } from '../../queries/emailBranding'

export type BrandingTemplateId = 'minimal' | 'standard' | 'compliance'

export const BRANDING_TEMPLATE_IDS: readonly BrandingTemplateId[] = [
  'minimal',
  'standard',
  'compliance',
]

// Mirror of the server's closed registry (internal/mailtemplates/tokens.go); the live list from the API wins.
export const KNOWN_BRANDING_VARIABLES: readonly string[] = [
  'brand_name',
  'support_email',
  'dashboard_url',
  'instance_name',
  'year',
]

export const HEX_COLOR_RE = /^#[0-9a-fA-F]{6}$/

export interface BrandingTemplateCopy {
  text: string
  address?: string
  dashboardLink: string
}

export interface BrandingTemplateContext {
  copy: BrandingTemplateCopy
  /** Variables the server accepts; defaults to the known registry. */
  variables?: readonly string[]
  /** Current accent color; a template only fills it when empty. */
  currentColor: string
  brandColor?: string
  /** https address of this instance's dashboard, when one is known. */
  dashboardUrl?: string
}

/** variablesIn lists the {{names}} a string uses. */
export function variablesIn(text: string): string[] {
  return Array.from(
    text.matchAll(/\{\{\s*([a-z_]+)\s*\}\}/g),
    (m) => m[1] ?? '',
  )
}

// A token outside the registry would be refused on save, so it is dropped here instead.
function sanitize(text: string, allowed: readonly string[]): string {
  return text
    .replace(/\{\{\s*([a-z_]+)\s*\}\}/g, (m, name: string) =>
      allowed.includes(name) ? m : '',
    )
    .replace(/\s{2,}/g, ' ')
    .trim()
}

const TEMPLATE_SHAPE: Record<
  BrandingTemplateId,
  { link: boolean; address: boolean }
> = {
  minimal: { link: false, address: false },
  standard: { link: true, address: false },
  compliance: { link: true, address: true },
}

/** buildBrandingTemplate returns the fields a template fills; the caller applies them as unsaved edits. */
export function buildBrandingTemplate(
  id: BrandingTemplateId,
  ctx: BrandingTemplateContext,
): Partial<EmailBranding> {
  const { link: withLink, address: withAddress } = TEMPLATE_SHAPE[id]
  const allowed = ctx.variables ?? KNOWN_BRANDING_VARIABLES
  const patch: Partial<EmailBranding> = {
    footer_text: sanitize(ctx.copy.text, allowed),
    footer_links:
      withLink && ctx.dashboardUrl?.startsWith('https://')
        ? [{ label: ctx.copy.dashboardLink, url: ctx.dashboardUrl }]
        : [],
  }
  if (withAddress) {
    patch.footer_address = sanitize(ctx.copy.address ?? '', allowed)
  }
  if (
    !ctx.currentColor &&
    ctx.brandColor &&
    HEX_COLOR_RE.test(ctx.brandColor)
  ) {
    patch.brand_color = ctx.brandColor
  }
  return patch
}
