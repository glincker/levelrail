// Circle SVGs are loaded as URLs, not React components: the Spanish component alone is about 90 kB
// because it embeds every shape, while its circle file is 2 kB.
import gb from '@thesvg/flags/svg/circle/gb.svg?url'
import de from '@thesvg/flags/svg/circle/de.svg?url'
import es from '@thesvg/flags/svg/circle/es.svg?url'
import fr from '@thesvg/flags/svg/circle/fr.svg?url'
import br from '@thesvg/flags/svg/circle/br.svg?url'
import cn from '@thesvg/flags/svg/circle/cn.svg?url'
import jp from '@thesvg/flags/svg/circle/jp.svg?url'

export interface LanguageFlag {
  /** Circle flag image URL. */
  flag: string
  /** The language written in itself. */
  native: string
  /** The language in English, shown as the muted second line. */
  english: string
}

// English uses the GB flag: the app copy is British-neutral and GB is the origin of the language.
// A new language is one line here plus its code in i18n/supported.ts.
export const LANGUAGE_FLAGS: Record<string, LanguageFlag> = {
  en: { flag: gb, native: 'English', english: 'English' },
  es: { flag: es, native: 'Español', english: 'Spanish' },
  de: { flag: de, native: 'Deutsch', english: 'German' },
  fr: { flag: fr, native: 'Français', english: 'French' },
  'pt-BR': {
    flag: br,
    native: 'Português (Brasil)',
    english: 'Portuguese (Brazil)',
  },
  ja: { flag: jp, native: '日本語', english: 'Japanese' },
  'zh-CN': { flag: cn, native: '中文 (简体)', english: 'Chinese (Simplified)' },
}

export function languageFlag(code: string): LanguageFlag | undefined {
  return LANGUAGE_FLAGS[code]
}
