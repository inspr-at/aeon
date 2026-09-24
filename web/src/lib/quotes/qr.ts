// SPDX-License-Identifier: AGPL-3.0-only
import qrcode from 'qrcode-generator'

export function quoteQr(url: string): { size: number; path: string } {
  const code = qrcode(0, 'M')
  code.addData(url)
  code.make()
  const size = code.getModuleCount()
  let path = ''
  for (let row = 0; row < size; row++) for (let col = 0; col < size; col++) {
    if (code.isDark(row, col)) path += `M${col} ${row}h1v1h-1z`
  }
  return { size, path }
}
