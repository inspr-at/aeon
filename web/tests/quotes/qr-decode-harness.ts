// SPDX-License-Identifier: AGPL-3.0-only
// Decode the rendered SVG with a separate reader, rather than trusting the
// encoder's input or comparing its own matrix to itself.
import jsQR from 'jsqr'

export async function decodeQuoteQr(selector: string): Promise<string | null> {
  const svg = document.querySelector<SVGSVGElement>(selector)
  if (!svg) return null
  const box = svg.viewBox.baseVal
  const scale = 8
  const canvas = document.createElement('canvas')
  canvas.width = box.width * scale
  canvas.height = box.height * scale
  const context = canvas.getContext('2d', { willReadFrequently: true })
  if (!context) return null
  context.fillStyle = '#fff'
  context.fillRect(0, 0, canvas.width, canvas.height)
  context.fillStyle = '#000'
  const path = svg.querySelector('path')?.getAttribute('d') ?? ''
  for (const match of path.matchAll(/M(\d+) (\d+)h1v1h-1z/g)) {
    context.fillRect((Number(match[1]) - box.x) * scale, (Number(match[2]) - box.y) * scale, scale, scale)
  }
  return jsQR(context.getImageData(0, 0, canvas.width, canvas.height).data, canvas.width, canvas.height)?.data ?? null
}
