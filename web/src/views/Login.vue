<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, loadSession, session } from '../api'

const router = useRouter()
const password = ref('')
const error = ref('')
const busy = ref(false)

// Web interface of an add-on served by the panel on another port of this
// host, which sent here to log in first.
function backTo() {
  try {
    const u = new URL(router.currentRoute.value.query.next || '')
    if (u.protocol === 'https:' && u.hostname === window.location.hostname) return u.href
  } catch {
    // no or bad address
  }
  return ''
}

async function submit() {
  busy.value = true
  error.value = ''
  try {
    await api('POST', '/login', { password: password.value })
    await loadSession()
    const next = backTo()
    if (next && !session.mustChange) window.location.assign(next)
    else router.push(session.mustChange ? '/password' : '/')
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="narrow card">
    <h1>{{ $t('login.title') }}</h1>
    <div v-if="error" class="alert error">{{ error }}</div>
    <form @submit.prevent="submit">
      <div class="field">
        <label for="password">{{ $t('login.password') }}</label>
        <input id="password" v-model="password" type="password" autocomplete="current-password" autofocus required>
      </div>
      <button class="primary" :disabled="busy">{{ $t('login.submit') }}</button>
    </form>
    <p class="muted small mt">{{ $t('login.hint', { user: session.user, password: 'supla' }) }}</p>
  </div>
</template>
