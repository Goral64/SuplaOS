import { createI18n } from 'vue-i18n'
import en from './locales/en.json'
import pl from './locales/pl.json'

export const languages = { en: 'English', pl: 'Polski' }

export function browserLanguage() {
  for (const l of navigator.languages || [navigator.language]) {
    const code = (l || '').slice(0, 2).toLowerCase()
    if (languages[code]) return code
  }
  return 'en'
}

export const i18n = createI18n({
  legacy: false,
  locale: browserLanguage(),
  fallbackLocale: 'en',
  messages: { en, pl },
})

export function setLanguage(lang) {
  if (languages[lang]) {
    i18n.global.locale.value = lang
    document.documentElement.lang = lang
  }
}
