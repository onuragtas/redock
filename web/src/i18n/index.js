import { createI18n } from 'vue-i18n'
import en from './locales/en.json'
import tr from './locales/tr.json'

const STORAGE_KEY = 'redock_locale'
const SUPPORTED = ['en', 'tr']

// Resolve the initial locale: saved preference → browser language → English.
function resolveInitialLocale() {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved && SUPPORTED.includes(saved)) return saved
  } catch (_) {
    /* localStorage may be unavailable */
  }
  const nav = (typeof navigator !== 'undefined' && (navigator.language || '')).toLowerCase()
  if (nav.startsWith('tr')) return 'tr'
  return 'en'
}

const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: resolveInitialLocale(),
  fallbackLocale: 'en',
  messages: { en, tr },
  // Missing keys fall back silently to the key/English instead of spamming the console.
  missingWarn: false,
  fallbackWarn: false
})

// Persist + apply a locale change from anywhere (e.g. the language switcher).
export function setLocale(locale) {
  if (!SUPPORTED.includes(locale)) return
  i18n.global.locale.value = locale
  try {
    localStorage.setItem(STORAGE_KEY, locale)
  } catch (_) {
    /* ignore */
  }
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('lang', locale)
  }
}

export const SUPPORTED_LOCALES = SUPPORTED
export default i18n
