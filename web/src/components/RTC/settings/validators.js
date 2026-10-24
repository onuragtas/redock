// Client-side checks that mirror the server's validation, so mistakes show
// next to the field while typing instead of as an error after saving.

const domainRe = /^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$/i
const ipv4Re = /^(25[0-5]|2[0-4]\d|1?\d?\d)(\.(25[0-5]|2[0-4]\d|1?\d?\d)){3}$/
const ipv6Re = /^[0-9a-f:]+(%\w+)?$/i

export const isDomain = (v) => domainRe.test(v)
export const isIPv4 = (v) => ipv4Re.test(v)
export const isIP = (v) => isIPv4(v) || (v.includes(':') && ipv6Re.test(v))

export const isPort = (n) => Number.isInteger(n) && n >= 1 && n <= 65535

// "host:port", "[v6]:port" or ":port"
export const isHostPort = (v) => {
  const m = String(v).trim().match(/^(\[[^\]]+\]|[^:]*):(\d+)$/)
  return !!m && isPort(Number(m[2]))
}

export const isCIDR = (v) => {
  const [ip, bits, extra] = v.split('/')
  if (extra !== undefined) return false
  if (!isIP(ip)) return false
  if (bits === undefined) return true
  const n = Number(bits)
  return Number.isInteger(n) && n >= 0 && n <= (isIPv4(ip) ? 32 : 128)
}

export const splitList = (text) => String(text || '').split(/[\s,]+/).map((v) => v.trim()).filter(Boolean)

// Port range check; allowZero accepts 0/0 as "any port".
export const rangeError = (min, max, t, allowZero = false) => {
  if (allowZero && !min && !max) return ''
  if (!isPort(min) || !isPort(max)) return t('settingsForm.errPortRange')
  if (min > max) return t('settingsForm.errMinMax')
  return ''
}
