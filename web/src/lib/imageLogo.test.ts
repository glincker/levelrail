import { describe, expect, it } from 'vitest'
import { logoIdForImage } from './imageLogo'

describe('logoIdForImage', () => {
  it.each([
    ['postgres:16', 'postgresql'],
    ['docker.io/library/redis:7-alpine', 'redis'],
    ['ghcr.io/n8n-io/n8n:latest', 'n8n'],
    ['mongo', 'mongodb'],
    ['node:20@sha256:abc', 'nodedotjs'],
    ['registry.example.com:5000/team/nginx:1.27', 'nginx'],
    ['ghcr.io/acme/my-private-app:v1', undefined],
    ['', undefined],
  ])('%s -> %s', (image, want) => {
    expect(logoIdForImage(image)).toBe(want)
  })
})
