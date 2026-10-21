import { reactive } from 'vue'

// The question shown in the confirm dialog (App.vue), instead of the
// browser's confirm() and alert().
export const dialog = reactive({ message: '', ok: '', danger: false, cancel: true, resolve: null })

// ask resolves with true when the user confirms. ok is the text of the
// confirm button; danger marks it red.
export function ask(message, { ok = '', danger = false } = {}) {
  return new Promise((resolve) => Object.assign(dialog, { message, ok, danger, cancel: true, resolve }))
}

// tell shows a message with one button.
export function tell(message) {
  return new Promise((resolve) => Object.assign(dialog, { message, ok: '', danger: false, cancel: false, resolve }))
}

export function answer(value) {
  const { resolve } = dialog
  dialog.resolve = null
  if (resolve) resolve(value)
}
