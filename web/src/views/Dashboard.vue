<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, formatBytes, text } from '../api'
import StateBadge from '../components/StateBadge.vue'

const { t, locale } = useI18n()
const info = ref(null)
const addons = ref([])
let timer = null

async function load() {
  const [i, a] = await Promise.all([api('GET', '/system'), api('GET', '/addons')])
  info.value = i
  addons.value = a.filter((x) => x.installed)
}
onMounted(() => {
  load()
  timer = setInterval(load, 10000)
})
onBeforeUnmount(() => clearInterval(timer))

const uptime = computed(() => {
  const s = info.value?.uptime || 0
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  const parts = []
  if (d) parts.push(t('dashboard.days', { n: d }))
  if (d || h) parts.push(t('dashboard.hours', { n: h }))
  parts.push(t('dashboard.minutes', { n: m }))
  return parts.join(' ')
})
const memUsed = computed(() => info.value && (1 - info.value.memFree / info.value.memTotal) * 100)
const diskUsed = computed(() => info.value && (1 - info.value.diskFree / info.value.diskTotal) * 100)

function webURL(a) {
  return `${a.web.scheme}://${window.location.hostname}:${a.web.port}${a.web.path || '/'}`
}
</script>

<template>
  <h1>{{ $t('dashboard.title') }}</h1>
  <div v-if="info?.rebootRequired" class="alert warn">
    {{ $t('updates.rebootRequired') }} <router-link to="/updates">{{ $t('nav.updates') }}</router-link>
  </div>
  <div v-if="info" class="grid">
    <div class="card">
      <div class="stat-label">{{ $t('dashboard.addresses') }}</div>
      <div class="stat-value">{{ info.addresses.join(', ') || '-' }}</div>
      <div class="muted small">{{ info.hostname }}.local</div>
    </div>
    <div class="card">
      <div class="stat-label">{{ $t('dashboard.uptime') }}</div>
      <div class="stat-value">{{ uptime }}</div>
      <div class="muted small">{{ $t('dashboard.load') }}: {{ (info.load || []).join(' ') }}</div>
      <div class="muted small">CPU: {{ info.cpus }}</div>
    </div>
    <div class="card">
      <div class="stat-label">{{ $t('dashboard.memory') }}</div>
      <div class="stat-value">{{ memUsed.toFixed(0) }}%</div>
      <div class="bar"><div :style="{ width: memUsed + '%' }"></div></div>
      <div class="muted small">{{ $t('dashboard.free', { free: formatBytes(info.memFree), total: formatBytes(info.memTotal) }) }}</div>
    </div>
    <div class="card">
      <div class="stat-label">{{ $t('dashboard.disk') }}</div>
      <div class="stat-value">{{ diskUsed.toFixed(0) }}%</div>
      <div class="bar"><div :style="{ width: diskUsed + '%' }"></div></div>
      <div class="muted small">{{ $t('dashboard.free', { free: formatBytes(info.diskFree), total: formatBytes(info.diskTotal) }) }}</div>
    </div>
    <div v-if="info.temperature != null" class="card">
      <div class="stat-label">{{ $t('dashboard.temperature') }}</div>
      <div class="stat-value">{{ info.temperature.toFixed(1) }} °C</div>
    </div>
    <div class="card">
      <div class="stat-label">{{ $t('dashboard.version') }}</div>
      <div class="stat-value">SuplaOS {{ info.version }}</div>
      <div class="muted small">{{ info.timezone }}</div>
    </div>
  </div>

  <h2 class="mt">{{ $t('dashboard.addons') }}</h2>
  <div v-if="!addons.length" class="card">
    <p>{{ $t('dashboard.noAddons') }}</p>
    <router-link to="/addons" class="button">{{ $t('dashboard.goAddons') }}</router-link>
  </div>
  <div v-for="a in addons" :key="a.id" class="card">
    <div class="row">
      <router-link :to="'/addons/' + a.id"><strong>{{ text(a.name, locale) }}</strong></router-link>
      <StateBadge v-for="c in a.containers" :key="c.Service" :container="c" />
      <span class="spacer"></span>
      <a v-if="a.web" class="button" :href="webURL(a)" target="_blank" rel="noopener">{{ $t('addons.open') }}</a>
    </div>
  </div>
</template>
