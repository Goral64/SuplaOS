<script setup>
import { watch } from 'vue'
import { useRouter } from 'vue-router'
import { api, session } from './api'
import { languages, setLanguage } from './i18n'
import JobDialog from './components/JobDialog.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'

const router = useRouter()

watch(() => session.lang, (l) => l && setLanguage(l), { immediate: true })

async function changeLanguage(e) {
  const lang = e.target.value
  setLanguage(lang)
  if (session.loggedIn) {
    await api('PUT', '/settings', { lang })
    session.lang = lang
  }
}

async function logout() {
  await api('POST', '/logout')
  session.loggedIn = false
  router.push('/login')
}
</script>

<template>
  <header class="topbar">
    <div class="topbar-inner">
      <router-link to="/" class="brand"><span class="brand-dot"></span>SuplaOS</router-link>
      <nav v-if="session.loggedIn && !session.mustChange && session.setupDone" class="nav">
        <router-link to="/" exact-active-class="router-link-active" active-class="">{{ $t('nav.dashboard') }}</router-link>
        <router-link to="/addons">{{ $t('nav.addons') }}</router-link>
        <router-link to="/updates">{{ $t('nav.updates') }}</router-link>
        <router-link to="/system">{{ $t('nav.system') }}</router-link>
      </nav>
      <span v-else class="spacer"></span>
      <div class="topbar-right">
        <select :value="$i18n.locale" :aria-label="$t('common.language')" style="width: auto" @change="changeLanguage">
          <option v-for="(name, code) in languages" :key="code" :value="code">{{ name }}</option>
        </select>
        <button v-if="session.loggedIn" class="link" @click="logout">{{ $t('nav.logout') }}</button>
      </div>
    </div>
  </header>
  <main>
    <router-view />
  </main>
  <JobDialog />
  <ConfirmDialog />
</template>
