<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'

// The page restarts the device, with its own question.
const emit = defineEmits(['reboot'])
const { t } = useI18n()
const view = ref(null)
const form = ref(null)
const error = ref('')
const message = ref('')
const busy = ref(false)
let timer

async function load(withForm) {
  view.value = await api('GET', '/fan')
  if (withForm) form.value = { ...view.value.settings }
}
onMounted(async () => {
  await load(true)
  timer = setInterval(() => load(false), 10000)
})
onUnmounted(() => clearInterval(timer))

const pi5 = computed(() => view.value?.model.startsWith('Raspberry Pi 5'))

async function save() {
  busy.value = true
  error.value = message.value = ''
  try {
    view.value = await api('PUT', '/fan', form.value)
    form.value = { ...view.value.settings }
    message.value = t('common.saved')
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="view && view.model" class="card">
    <h2>{{ $t('fan.title') }}</h2>
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-if="message && !view.rebootNeeded" class="alert ok">{{ message }}</div>
    <div v-if="view.rebootNeeded" class="alert warn row">
      <span>{{ $t('fan.rebootNeeded') }}</span>
      <span class="spacer"></span>
      <button @click="emit('reboot')">{{ $t('system.reboot') }}</button>
    </div>
    <p class="muted">
      {{ view.model }} · {{ $t('fan.temperature', { temp: view.temperature.toFixed(1) }) }}
      <template v-if="view.settings.kind === 'argon' && !view.rebootNeeded"> · {{ $t('fan.speed', { speed: view.speed }) }}</template>
    </p>
    <div v-if="view.error && view.settings.kind === 'argon' && !view.rebootNeeded" class="alert error">{{ $t('fan.error', { error: view.error }) }}</div>
    <p v-if="pi5" class="muted small">{{ $t('fan.pi5') }}</p>

    <form @submit.prevent="save">
      <div class="field">
        <label for="fan-kind">{{ $t('fan.kind') }}</label>
        <select id="fan-kind" v-model="form.kind">
          <option value="none">{{ $t('fan.none') }}</option>
          <option value="argon">{{ pi5 ? 'Argon ONE V3' : 'Argon ONE (V2, M.2)' }}</option>
          <option value="gpio">{{ $t('fan.gpio') }}</option>
        </select>
      </div>

      <template v-if="form.kind === 'argon'">
        <div class="row">
          <div class="field">
            <label for="fan-start">{{ $t('fan.start') }}</label>
            <input id="fan-start" v-model.number="form.start" type="number" min="30" max="84" required style="max-width: 7rem">
          </div>
          <div class="field">
            <label for="fan-full">{{ $t('fan.full') }}</label>
            <input id="fan-full" v-model.number="form.full" type="number" min="31" max="85" required style="max-width: 7rem">
          </div>
        </div>
        <div class="help field">{{ $t('fan.argonHelp') }}</div>
        <div class="field">
          <label class="check"><input v-model="form.button" type="checkbox"> {{ $t('fan.button') }}</label>
          <div class="help">{{ $t('fan.buttonHelp') }}</div>
        </div>
      </template>

      <template v-if="form.kind === 'gpio'">
        <div class="row">
          <div class="field">
            <label for="fan-pin">{{ $t('fan.pin') }}</label>
            <input id="fan-pin" v-model.number="form.pin" type="number" min="2" max="27" required style="max-width: 7rem">
          </div>
          <div class="field">
            <label for="fan-temp">{{ $t('fan.on') }}</label>
            <input id="fan-temp" v-model.number="form.temp" type="number" min="30" max="85" required style="max-width: 7rem">
          </div>
        </div>
        <div class="help field">{{ $t('fan.gpioHelp') }}</div>
      </template>

      <button class="primary" :disabled="busy">{{ $t('common.save') }}</button>
    </form>
  </div>
</template>
