<script setup>
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { ask } from './dialog'

// Limits of the Supla Cloud accounts, set to a preset with one click.
const { t } = useI18n()
const presets = ['default', 'big', 'large', 'max']
const accounts = ref(null)
const presetValues = ref({})
const choice = ref({})
const open = ref({})
const error = ref('')
const done = ref('')
const busy = ref(false)

async function load() {
  try {
    const view = await api('GET', '/supla/limits')
    accounts.value = view.accounts
    presetValues.value = view.presets
    for (const a of accounts.value) choice.value[a.id] ??= a.preset || 'large'
    error.value = ''
  } catch (e) {
    error.value = e.message
  }
}
onMounted(load)

// Limits the preset puts below what the account already has.
function below(a, preset) {
  return a.limits.filter((l) => l.used != null && l.used > presetValue(preset, l.name))
}
const presetValue = (preset, name) => presetValues.value[preset]?.[name] ?? Infinity

async function apply(a) {
  const preset = choice.value[a.id]
  const over = below(a, preset)
  const text = t('accountLimits.confirm', { email: a.email, preset: t('accountLimits.preset.' + preset) }) +
    (over.length ? ' ' + t('accountLimits.below', { names: over.map((l) => t('accountLimits.name.' + l.name)).join(', ') }) : '')
  if (!(await ask(text, { ok: t('accountLimits.apply') }))) return
  busy.value = true
  error.value = ''
  done.value = ''
  try {
    await api('PUT', `/supla/limits/${a.id}`, { preset })
    done.value = t('accountLimits.done', { email: a.email })
    await load()
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="accounts?.length || error" class="card">
    <h2>{{ $t('accountLimits.title') }}</h2>
    <p class="muted">{{ $t('accountLimits.intro') }}</p>
    <div v-if="error" class="alert error">{{ error }}</div>
    <p v-if="done" class="alert ok">{{ done }}</p>

    <div v-for="a in accounts" :key="a.id" class="account">
      <div class="row">
        <strong>{{ a.email }}</strong>
        <span class="badge" :class="{ ok: a.preset === 'large' || a.preset === 'max' }">
          {{ a.preset ? $t('accountLimits.preset.' + a.preset) : $t('accountLimits.custom') }}
        </span>
        <span class="spacer"></span>
        <select v-model="choice[a.id]" :aria-label="$t('accountLimits.title')">
          <option v-for="p in presets" :key="p" :value="p">{{ $t('accountLimits.preset.' + p) }}</option>
        </select>
        <button type="button" class="primary" :disabled="busy || choice[a.id] === a.preset" @click="apply(a)">
          {{ $t('accountLimits.apply') }}
        </button>
      </div>
      <button type="button" class="link small" @click="open[a.id] = !open[a.id]">
        {{ open[a.id] ? $t('accountLimits.hide') : $t('accountLimits.show') }}
      </button>
      <table v-if="open[a.id]" class="small">
        <tr v-for="l in a.limits" :key="l.name">
          <td>{{ $t('accountLimits.name.' + l.name) }}</td>
          <td class="num" :class="{ over: l.used != null && l.used >= l.limit }">
            {{ l.used != null ? `${l.used} / ${l.limit}` : l.limit }}
          </td>
        </tr>
      </table>
    </div>
    <p class="muted small">{{ $t('accountLimits.help') }}</p>
  </div>
</template>

<style scoped>
.account { padding: .6rem 0; border-bottom: 1px solid var(--border); }
.account select { width: auto; }
.account table { margin-top: .3rem; max-width: 30rem; }
.num { text-align: right; white-space: nowrap; }
.over { color: var(--danger); font-weight: 600; }
</style>
