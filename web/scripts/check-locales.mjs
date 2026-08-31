// Compiles every translation message so a broken one fails the build instead of
// surfacing as a runtime "SyntaxError: Invalid linked format" in the console.
//
// vue-i18n gives some characters meaning inside a message:
//   @   starts a linked message  → a literal one must be written {'@'}
//   |   separates plural forms
//   {}  interpolation
// An address like alici@example.com therefore has to be escaped, which is easy
// to forget and impossible to notice until the message is rendered.

import { readFileSync, readdirSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createI18n } from 'vue-i18n'

const localeDir = join(dirname(fileURLToPath(import.meta.url)), '..', 'src', 'i18n', 'locales')

// Placeholder values so interpolation does not fail for its own reasons.
const sampleParams = new Proxy({}, { get: () => 'x' })

let failures = 0

for (const file of readdirSync(localeDir).filter((name) => name.endsWith('.json'))) {
  const locale = file.replace(/\.json$/, '')
  const messages = JSON.parse(readFileSync(join(localeDir, file), 'utf8'))

  const i18n = createI18n({ legacy: false, locale, messages: { [locale]: messages }, warnHtmlMessage: false })

  const walk = (node, prefix) => {
    for (const [key, value] of Object.entries(node)) {
      const path = prefix ? `${prefix}.${key}` : key

      if (value && typeof value === 'object') {
        walk(value, path)
        continue
      }
      if (typeof value !== 'string') continue

      try {
        i18n.global.t(path, sampleParams)
      } catch (error) {
        failures++
        console.error(`✖ ${locale}: ${path}\n  ${value}\n  ${error.message}`)
        if (value.includes('@') && !value.includes("{'@'}")) {
          console.error("  hint: write a literal @ as {'@'}")
        }
      }
    }
  }

  walk(messages, '')
}

if (failures > 0) {
  console.error(`\n${failures} translation message(s) will fail at runtime.`)
  process.exit(1)
}

console.log('All translation messages compile.')
