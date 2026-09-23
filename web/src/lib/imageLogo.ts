import { TEMPLATE_LOGO_LOADERS } from './templateLogos'

const IMAGE_ALIASES: Record<string, string> = {
  postgres: 'postgresql',
  mongo: 'mongodb',
  node: 'nodedotjs',
  httpd: 'apache',
  'php-fpm': 'php',
  openjdk: 'openjdk',
  'eclipse-temurin': 'openjdk',
}

// Maps a container image reference (registry/namespace/name:tag) to a
// logo id known to TemplateLogo, or undefined when there is none.
export function logoIdForImage(image: string): string | undefined {
  const name = image
    .split('@')[0]
    ?.split('/')
    .pop()
    ?.split(':')[0]
    ?.toLowerCase()
  if (!name) {
    return undefined
  }
  const id = IMAGE_ALIASES[name] ?? name
  return id in TEMPLATE_LOGO_LOADERS ? id : undefined
}
