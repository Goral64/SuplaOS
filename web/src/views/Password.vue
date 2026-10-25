<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api, session } from '../api'

const router = useRouter()
const { t } = useI18n()
const current = ref('')
const next = ref('')
const repeat = ref('')
const error = ref('')
const message = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  message.value = ''
  if (next.value.length < 8) return (error.value = t('password.tooShort'))
  if (next.value !== repeat.value) return (error.value = t('password.mismatch'))
  busy.value = true
  try {
    await api('POST', '/password', { current: current.value, new: next.value })
    const forced = session.mustChange
    session.mustChange = false
    current.value = next.value = repeat.value = ''
    if (forced) router.push(session.setupDone ? '/' : '/setup')
    else message.value = t('password.changed')
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="narrow card">
    <h1>{{ $t('password.title') }}</h1>
    <div v-if="session.mustChange" class="alert warn">{{ $t('password.forced', { user: session.user }) }}</div>
    <div v-if="error" class="alert error">{{ error }}</div>
    <div v-if="message" class="alert ok">{{ message }}</div>
    <form @submit.prevent="submit">
      <div class="field">
        <label for="current">{{ $t('password.current') }}</label>
        <input id="current" v-model="current" type="password" autocomplete="current-password" required>
      </div>
      <div class="field">
        <label for="new">{{ $t('password.new') }}</label>
        <input id="new" v-model="next" type="password" autocomplete="new-password" required>
        <div class="help">{{ $t('password.tooShort') }}</div>
      </div>
      <div class="field">
        <label for="repeat">{{ $t('password.repeat') }}</label>
        <input id="repeat" v-model="repeat" type="password" autocomplete="new-password" required>
      </div>
      <div class="row">
        <button class="primary" :disabled="busy">{{ $t('common.save') }}</button>
        <router-link v-if="!session.mustChange" to="/system/access" class="button">{{ $t(message ? 'common.back' : 'common.cancel') }}</router-link>
      </div>
    </form>
  </div>
</template>
