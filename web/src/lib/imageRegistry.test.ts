import { describe, expect, it } from 'vitest'
import { registryLinkForImage } from './imageRegistry'

describe('registryLinkForImage', () => {
  it.each([
    ['postgres:16', 'https://hub.docker.com/_/postgres'],
    ['library/redis:7', 'https://hub.docker.com/_/redis'],
    ['grafana/grafana:11.1.0', 'https://hub.docker.com/r/grafana/grafana'],
    ['docker.io/n8nio/n8n', 'https://hub.docker.com/r/n8nio/n8n'],
    [
      'ghcr.io/immich-app/immich-server:v1',
      'https://github.com/immich-app?tab=packages&q=immich-server',
    ],
    [
      'quay.io/keycloak/keycloak:25',
      'https://quay.io/repository/keycloak/keycloak',
    ],
    [
      'lscr.io/linuxserver/sonarr:latest',
      'https://hub.docker.com/r/linuxserver/sonarr',
    ],
    ['localhost:5000/app:1', null],
    ['registry.example.com/team/app', null],
    ['${IMAGE}', null],
  ])('%s', (image, url) => {
    expect(registryLinkForImage(image)?.url ?? null).toBe(url)
  })
})
