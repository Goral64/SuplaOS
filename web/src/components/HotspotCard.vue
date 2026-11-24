<script setup>
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { ask } from './dialog'

const { t } = useI18n()
const hotspot = ref(null)
const error = ref('')
const busy = ref(false)

async function load() {
  hotspot.value = await api('GET', '/hotspot')
}
onMounted(load)

async function call(fn) {
  busy.value = true
  error.value = ''
  try {
    await fn()
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}

const setOn = (e) => call(async () => {
  const on = e.target.checked
  if (!on && hotspot.value.active && !(await ask(t('hotspot.offActive', { ssid: hotspot.value.ssid }), { ok: t('hotspot.turnOff'), danger: true }))) {
    e.target.checked = true
    return
  }
  hotspot.value = await api('PUT', '/hotspot', { off: !on })
})

const newPassword = () => call(async () => {
  if (!(await ask(t('hotspot.newConfirm'), { ok: t('hotspot.new') }))) return
  hotspot.value = await api('POST', '/hotspot/password')
})
</script>

<template>
  <div v-if="hotspot && hotspot.device && hotspot.setupDone" class="card">
    <h2>{{ $t('hotspot.title') }}</h2>
    <p>{{ $t('hotspot.intro', { ssid: hotspot.ssid }) }}</p>
    <div v-if="error" class="alert error">{{ error }}</div>
    <label class="check"><input type="checkbox" :checked="!hotspot.off" :disabled="busy" @change="setOn"> {{ $t('hotspot.on') }}</label>
    <template v-if="!hotspot.off">
      <p class="mt">{{ $t('hotspot.network') }} <code>{{ hotspot.ssid }}</code><br>{{ $t('hotspot.password') }} <code>{{ hotspot.password }}</code></p>
      <p class="muted small">{{ $t('hotspot.passwordHelp') }}</p>
      <button :disabled="busy" @click="newPassword">{{ $t('hotspot.new') }}</button>
    </template>
    <p v-else class="muted small mt">{{ $t('hotspot.offHelp') }}</p>
  </div>
</template>
