<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'

// Shown after Restart and Shut down: waits until the system is back, with
// another boot ID, and opens the panel then.
const props = defineProps({ action: String })
const route = useRoute()
const reboot = computed(() => props.action === 'reboot')
// The panel stopped answering: the system went down.
const down = ref(false)
const progress = ref(0)
const slow = ref(false)
const started = Date.now()
let timer = null

async function bootID() {
  const ctl = new AbortController()
  const t = setTimeout(() => ctl.abort(), 3000)
  try {
    const r = await fetch('/api/session', { signal: ctl.signal, cache: 'no-store' })
    return r.ok ? (await r.json()).boot : null
  } catch {
    return null
  } finally {
    clearTimeout(t)
  }
}

async function poll() {
  const boot = await bootID()
  if (boot && (route.query.boot ? boot !== route.query.boot : down.value)) {
    progress.value = 100
    // A full load: the system may have come back with a new panel.
    window.location.assign('/')
    return
  }
  if (!boot) down.value = true
  // Not a real progress: it slows down near the end instead of stopping.
  const s = (Date.now() - started) / 1000
  progress.value = 95 * (1 - Math.exp(-s / (reboot.value ? 40 : 8)))
  slow.value = reboot.value && s > 300
  timer = setTimeout(poll, down.value && !reboot.value ? 5000 : 2000)
}
onMounted(() => { timer = setTimeout(poll, 2000) })
onUnmounted(() => clearTimeout(timer))
</script>

<template>
  <div class="card narrow-card">
    <template v-if="reboot">
      <h1>{{ $t('power.rebootTitle') }}</h1>
      <p>{{ $t('power.rebootText') }}</p>
      <div class="progress mt"><div :style="{ width: progress + '%' }"></div></div>
      <p v-if="slow" class="alert warn mt">{{ $t('power.slow') }}</p>
    </template>
    <template v-else-if="!down">
      <h1>{{ $t('power.poweroffTitle') }}</h1>
      <p>{{ $t('power.stopping') }}</p>
      <div class="progress mt"><div :style="{ width: progress + '%' }"></div></div>
    </template>
    <template v-else>
      <h1>{{ $t('power.offTitle') }}</h1>
      <p>{{ $t('power.offText') }}</p>
      <p class="muted small">{{ $t('power.waiting') }}</p>
    </template>
  </div>
</template>
