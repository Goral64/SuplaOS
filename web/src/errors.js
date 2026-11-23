import { i18n } from './i18n'
import pl from './locales/errors.pl.json'

// Error messages come from the panel in English. translateError shows them
// in the language of the interface: each message of the catalog is the
// format string of the Go code, its verbs ({0}, {1}...) are translated in
// turn, as they are often errors themselves. What is not in the catalog
// (output of docker, of Supla Cloud...) stays as it is.
const catalogs = { pl }
const compiled = {}

function compile(lang) {
  if (compiled[lang]) return compiled[lang]
  const list = Object.entries(catalogs[lang]?.messages || {}).map(([en, text]) => {
    let re = ''
    let rest = en
    // Verbs: %w and %v hold whole errors, the others a value.
    const verb = /%(#?[a-z])/
    let m
    while ((m = verb.exec(rest))) {
      re += rest.slice(0, m.index).replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
      re += m[1] === 'd' ? '(-?\\d+)' : m[1] === 'w' || m[1] === 'v' ? '(.+)' : '(.+?)'
      rest = rest.slice(m.index + m[0].length)
    }
    re += rest.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    // The most text outside the verbs first: the most specific message.
    return { re: new RegExp('^' + re + '$', 's'), text, weight: en.replace(/%#?[a-z]/g, '').length }
  })
  list.sort((a, b) => b.weight - a.weight)
  compiled[lang] = list
  return list
}

// localError translates a message into the language of the interface.
export const localError = (message) => translateError(message, i18n.global.locale.value)

export function translateError(message, lang) {
  if (!message || !catalogs[lang]) return message
  return translate(String(message), compile(lang), 0)
}

function translate(s, list, depth) {
  if (depth > 8) return s
  for (const { re, text } of list) {
    const m = re.exec(s)
    if (m) return text.replace(/\{(\d)\}/g, (_, i) => translate(m[+i + 1] ?? '', list, depth + 1))
  }
  // A message wrapped in one that is not in the catalog ("docker compose
  // up: ..."): the part after it may be.
  const i = s.indexOf(': ')
  if (i > 0) return s.slice(0, i + 2) + translate(s.slice(i + 2), list, depth + 1)
  return s
}
