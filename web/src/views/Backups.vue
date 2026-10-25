<script setup>
import { ref } from 'vue'
import FullBackupCard from '../components/FullBackupCard.vue'
import AutoBackupCard from '../components/AutoBackupCard.vue'

const fullBackups = ref(null)
</script>

<template>
  <h1>{{ $t('backups.title') }}</h1>
  <FullBackupCard ref="fullBackups" />
  <AutoBackupCard @done="fullBackups?.load()" />
  <p class="muted small">{{ $t('backups.addons') }}</p>
</template>
