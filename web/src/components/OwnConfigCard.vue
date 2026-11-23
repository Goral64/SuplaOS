<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api'
import { runJob } from './jobs'

const props = defineProps({ id: { type: String, required: true } })
const emit = defineEmits(['saved'])
const env = ref('')
const compose = ref('')
const error = ref('')
const busy = ref(false)

async function load() {
  const own = await api('GET', `/addons/${props.id}/own`)
  env.value = own.env
  compose.value = own.compose
}
onMounted(() => load().catch((e) => { error.value = e.message }))

async function save() {
  busy.value = true
  error.value = ''
  try {
    const job = await runJob('PUT', `/addons/${props.id}/own`, { env: env.value, compose: compose.value })
    if (job?.state === 'failed') error.value = job.error
    await load()
    emit('saved')
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <details class="card">
    <summary><h2 style="display: inline; margin: 0">{{ $t('own.title') }}</h2></summary>
    <p class="mt">{{ $t('own.intro') }}</p>
    <div v-if="error" class="alert error">{{ error }}</div>
    <form @submit.prevent="save">
      <div class="field">
        <label for="own-env">{{ $t('own.env') }}</label>
        <textarea id="own-env" v-model="env" rows="5" spellcheck="false" placeholder="KEY=value"></textarea>
        <div class="help">{{ $t('own.envHelp') }}</div>
      </div>
      <div class="field">
        <label for="own-compose">{{ $t('own.compose') }}</label>
        <textarea id="own-compose" v-model="compose" rows="10" spellcheck="false" placeholder="services:"></textarea>
        <div class="help">{{ $t('own.composeHelp') }}</div>
      </div>
      <button class="primary" :disabled="busy">{{ $t('own.save') }}</button>
    </form>
  </details>
</template>
