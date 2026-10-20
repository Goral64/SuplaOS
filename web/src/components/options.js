// active tells whether an option applies, like Condition.Match in Go:
// installed is the list of installed add-on ids, values the form values.
export function active(option, values, installed) {
  const w = option.when
  if (!w) return true
  if (w.installed && !installed.includes(w.installed)) return false
  if (w.notInstalled && installed.includes(w.notInstalled)) return false
  if (w.option) {
    // Options of other add-ons ("<add-on>:<KEY>") are left to the server.
    if (w.option.includes(':')) return true
    const v = values[w.option]
    if (w.in ? !w.in.includes(v) : v !== w.equals) return false
  }
  return true
}

// formValues starts a form: stored values of an installed add-on, defaults
// of a new one.
export function formValues(addon) {
  const v = {}
  for (const o of addon.options) {
    v[o.key] = addon.installed ? (addon.values[o.key] ?? '') : o.default
  }
  return v
}
