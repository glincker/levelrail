// Mirrors spec.PublishedPort (internal/api app resources and compose deploy
// services[]). Response and app.yaml share this shape.
export type PublishedPortProtocol = 'tcp' | 'udp'

export interface PublishedPort {
  container_port: number
  host_port: number
  protocol: PublishedPortProtocol
}

export function formatPublishedPort(p: PublishedPort): string {
  return `${p.host_port}/${p.protocol}`
}
