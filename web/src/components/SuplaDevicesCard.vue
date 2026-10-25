<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'

// The CA certificate devices paste as Custom CA: path of the download in
// the API.
const props = defineProps({ path: { type: String, required: true }, server: { type: String, default: '' } })
const pem = ref('')
const copied = ref(false)

onMounted(async () => {
  const text = await api('GET', props.path).catch(() => '')
  pem.value = typeof text === 'string' && text.startsWith('-----BEGIN CERTIFICATE-----') ? text : ''
})

async function copy() {
  await navigator.clipboard.writeText(pem.value)
  copied.value = true
  setTimeout(() => { copied.value = false }, 2000)
}
</script>

<template>
  <div v-if="pem" class="card">
    <h2>{{ $t('suplaDevices.title') }}</h2>
    <p>{{ $t('suplaDevices.intro', { server: props.server }) }}</p>
    <div class="row">
      <button class="primary" @click="copy">{{ copied ? $t('suplaDevices.copied') : $t('suplaDevices.copy') }}</button>
      <a class="button" :href="'/api' + props.path" download="supla-ca.crt">{{ $t('common.download') }}</a>
    </div>
    <details class="mt">
      <summary class="small">{{ $t('suplaDevices.show') }}</summary>
      <pre class="log pem">{{ pem }}</pre>
    </details>
    <p class="muted small mt">{{ $t('suplaDevices.help') }}</p>
  </div>
</template>
