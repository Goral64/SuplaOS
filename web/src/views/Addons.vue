<script setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, text } from '../api'
import StateBadge from '../components/StateBadge.vue'

const { locale } = useI18n()
const addons = ref([])
onMounted(async () => (addons.value = await api('GET', '/addons')))
const installed = computed(() => addons.value.filter((a) => a.installed))
const available = computed(() => addons.value.filter((a) => !a.installed))
const installedIds = computed(() => installed.value.map((a) => a.id))
const missing = (a) => a.requires.filter((r) => !installedIds.value.includes(r))
</script>

<template>
  <h1>{{ $t('addons.title') }}</h1>
  <template v-if="installed.length">
    <h2>{{ $t('addons.installed') }}</h2>
    <div class="grid addon-list">
      <router-link v-for="a in installed" :key="a.id" :to="'/addons/' + a.id" class="card" style="color: inherit; text-decoration: none">
        <h2>{{ text(a.name, locale) }}</h2>
        <p class="muted">{{ text(a.description, locale) }}</p>
        <div class="row"><StateBadge v-for="c in a.containers" :key="c.Service" :container="c" /></div>
      </router-link>
    </div>
  </template>
  <template v-if="available.length">
    <h2 class="mt">{{ $t('addons.available') }}</h2>
    <div class="grid addon-list">
      <router-link v-for="a in available" :key="a.id" :to="'/addons/' + a.id" class="card" style="color: inherit; text-decoration: none">
        <h2>{{ text(a.name, locale) }}</h2>
        <p class="muted">{{ text(a.description, locale) }}</p>
        <div v-if="missing(a).length" class="small muted">{{ $t('addons.requires', { list: a.requires.join(', ') }) }}</div>
        <div v-else-if="a.recommends.length" class="small muted">{{ $t('addons.worksWith', { list: a.recommends.join(', ') }) }}</div>
      </router-link>
    </div>
  </template>
</template>
