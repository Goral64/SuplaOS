<script setup>
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, text } from '../api'
import { ask } from './dialog'

const { t, locale } = useI18n()
const view = ref(null)
const error = ref('')
const busy = ref(false)

async function load() {
  view.value = await api('GET', '/internet')
}
onMounted(load)

const panelOn = () => view.value.interfaces.find((i) => i.id === 'panel').allowed

async function toggle(i) {
  // Turned off from the internet, the panel is gone at once.
  if (i.id === 'panel' && !i.allowed && view.value.fromInternet &&
    !(await ask(t('internet.offConfirm'), { ok: t('internet.turnOff'), danger: true }))) {
    i.allowed = true
    return
  }
  busy.value = true
  error.value = ''
  try {
    await api('PUT', '/internet', { allowed: view.value.interfaces.filter((x) => x.allowed).map((x) => x.id) })
  } catch (e) {
    error.value = e.message
    await load()
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="view" class="card">
    <h2>{{ $t('internet.title') }}</h2>
    <div v-if="error" class="alert error">{{ error }}</div>
    <p>{{ $t('internet.intro') }}</p>
    <div v-for="i in view.interfaces" :key="i.id" class="field">
      <label class="check">
        <input v-model="i.allowed" type="checkbox" :disabled="busy || (i.id !== 'panel' && !panelOn())" @change="toggle(i)">
        {{ text(i.label, locale) }}
      </label>
    </div>
    <div class="help">{{ $t('internet.help') }}</div>
  </div>
</template>
