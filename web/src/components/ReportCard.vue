<script setup>
import { ref } from 'vue'
import { api } from '../api'

// A text about the device to paste when asking for help.
const text = ref('')
const error = ref('')
const busy = ref(false)
const copied = ref(false)

async function make() {
  busy.value = true
  error.value = ''
  try {
    text.value = (await api('GET', '/report')).text
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}

async function copy() {
  await navigator.clipboard.writeText(text.value)
  copied.value = true
  setTimeout(() => { copied.value = false }, 2000)
}
</script>

<template>
  <div class="card">
    <h2>{{ $t('report.title') }}</h2>
    <p>{{ $t('report.intro') }}</p>
    <div v-if="error" class="alert error">{{ error }}</div>
    <div class="row">
      <button :class="{ primary: !text }" :disabled="busy" @click="make">{{ text ? $t('report.again') : $t('report.make') }}</button>
      <button v-if="text" class="primary" @click="copy">{{ copied ? $t('report.copied') : $t('report.copy') }}</button>
    </div>
    <template v-if="text">
      <p class="muted small mt">{{ $t('report.check') }}</p>
      <pre class="log">{{ text }}</pre>
    </template>
  </div>
</template>
