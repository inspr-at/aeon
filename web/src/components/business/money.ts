// SPDX-License-Identifier: AGPL-3.0-only
// Exact decimal and integer minor-unit formatting. Never accepts a JS number:
// binary floats are not money. Callers keep the JSON spelling via parseJSONExact.

const DECIMAL = /^(0|[1-9]\d*)(?:\.(\d+))?$/
const MINOR = /^(0|[1-9]\d*)$/
const CURRENCY = /^[A-Z]{3}$/
const MAX_DIGITS = 40

// ISO 4217 minor-unit scales this shell is willing to apply. Unknown currencies
// throw so a display cannot invent a scale.
const MINOR_SCALE: Record<string, number> = {
  AUD: 2, BRL: 2, CAD: 2, CHF: 2, CNY: 2, DKK: 2, EUR: 2, GBP: 2, INR: 2, NOK: 2, SEK: 2, USD: 2, ZAR: 2,
  BHD: 3, JOD: 3, KWD: 3, OMR: 3, TND: 3,
  CLP: 0, ISK: 0, JPY: 0, KRW: 0, VND: 0, XAF: 0, XOF: 0,
}

export interface ExactDecimal { scale: number; units: bigint }

export function minorScale(currency: string): number | null {
  const code = currencyCode(currency)
  return Object.prototype.hasOwnProperty.call(MINOR_SCALE, code) ? MINOR_SCALE[code]! : null
}

export function parseDecimal(value: string): ExactDecimal {
  if (typeof value !== 'string') throw new Error('Amount must be an exact decimal string.')
  const text = value.trim()
  const match = DECIMAL.exec(text)
  if (!match) throw new Error('Amount must be an exact decimal string.')
  const whole = match[1]!
  const frac = match[2] ?? ''
  if (whole.length + frac.length > MAX_DIGITS) throw new Error('Amount is too large.')
  const digits = `${whole}${frac}`.replace(/^0+(?=\d)/, '')
  return { scale: frac.length, units: BigInt(digits) }
}

export function formatDecimal(amount: string, currency: string): string {
  const parsed = parseDecimal(amount)
  return `${renderUnits(parsed.units, parsed.scale)} ${currencyCode(currency)}`
}

export function formatMinor(minor: string, currency: string, scale?: number | null): string {
  if (typeof minor !== 'string') throw new Error('Minor units must be an integer string.')
  const text = minor.trim()
  if (!MINOR.test(text) || text.length > MAX_DIGITS) throw new Error('Minor units must be an integer string.')
  const code = currencyCode(currency)
  const resolved = scale === undefined ? minorScale(code) : scale
  if (resolved == null || !Number.isSafeInteger(resolved) || resolved < 0 || resolved > 8) {
    throw new Error('Unknown currency scale.')
  }
  return `${renderUnits(BigInt(text), resolved)} ${code}`
}

export function addDecimal(left: string, right: string): string {
  const a = parseDecimal(left)
  const b = parseDecimal(right)
  const scale = Math.max(a.scale, b.scale)
  return canonical(rescale(a, scale) + rescale(b, scale), scale)
}

export function multiplyDecimal(left: string, right: string): string {
  const a = parseDecimal(left)
  const b = parseDecimal(right)
  if (a.scale + b.scale > MAX_DIGITS) throw new Error('Amount is too large.')
  return canonical(a.units * b.units, a.scale + b.scale)
}

// Quote JSON numbers as their exact spelling before JSON.parse turns them into floats.
export function parseJSONExact(text: string): unknown {
  let quoted: string
  try {
    quoted = quoteJsonNumbers(text)
  } catch (error) {
    if (error instanceof Error && error.message === 'Exponent amounts are not exact decimals.') throw error
    throw new Error('JSON was not usable.')
  }
  try {
    return JSON.parse(quoted)
  } catch {
    throw new Error('JSON was not usable.')
  }
}

export async function readExactJSON(response: Response): Promise<unknown> {
  return parseJSONExact(await response.text())
}

function currencyCode(currency: string): string {
  if (typeof currency !== 'string' || !CURRENCY.test(currency)) throw new Error('Currency must be a three-letter code.')
  return currency
}

function rescale(value: ExactDecimal, scale: number): bigint {
  return value.units * 10n ** BigInt(scale - value.scale)
}

function canonical(units: bigint, scale: number): string {
  const digits = units.toString().padStart(scale + 1, '0')
  if (scale === 0) return digits
  return `${digits.slice(0, -scale)}.${digits.slice(-scale)}`
}

function renderUnits(units: bigint, scale: number): string {
  return group(canonical(units, scale))
}

function group(canonicalDecimal: string): string {
  const [whole, frac] = canonicalDecimal.split('.')
  const grouped = whole!.replace(/\B(?=(\d{3})+(?!\d))/g, '\u202f')
  return frac === undefined ? grouped : `${grouped}.${frac}`
}

function quoteJsonNumbers(text: string): string {
  let out = ''
  let i = 0
  while (i < text.length) {
    const c = text[i]!
    if (c === '"') {
      const start = i
      i += 1
      while (i < text.length) {
        if (text[i] === '\\') {
          i += 2
          continue
        }
        if (text[i] === '"') {
          i += 1
          break
        }
        i += 1
      }
      out += text.slice(start, i)
      continue
    }
    if (c === '-' || isDigit(c)) {
      const start = i
      if (c === '-') i += 1
      if (!isDigit(text[i] ?? '')) throw new Error('JSON was not usable.')
      if (text[i] === '0') i += 1
      else while (isDigit(text[i] ?? '')) i += 1
      if (text[i] === '.') {
        i += 1
        if (!isDigit(text[i] ?? '')) throw new Error('JSON was not usable.')
        while (isDigit(text[i] ?? '')) i += 1
      }
      const exponent = text[i]
      if (exponent === 'e' || exponent === 'E') throw new Error('Exponent amounts are not exact decimals.')
      out += JSON.stringify(text.slice(start, i))
      continue
    }
    out += c
    i += 1
  }
  return out
}

function isDigit(char: string): boolean {
  return char >= '0' && char <= '9'
}
