import { createRouter, createWebHistory } from 'vue-router'
import { session, loadSession } from './api'
import Login from './views/Login.vue'
import Password from './views/Password.vue'
import Setup from './views/Setup.vue'
import Dashboard from './views/Dashboard.vue'
import Addons from './views/Addons.vue'
import Addon from './views/Addon.vue'
import System from './views/System.vue'
import Updates from './views/Updates.vue'
import Backups from './views/Backups.vue'
import Power from './views/Power.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: Login, meta: { public: true } },
    { path: '/password', component: Password },
    { path: '/setup', component: Setup },
    { path: '/', component: Dashboard },
    { path: '/addons', component: Addons },
    { path: '/addons/:id', component: Addon, props: true },
    { path: '/system', redirect: '/system/general' },
    { path: '/system/:tab(general|network|access)', component: System, props: true },
    { path: '/backups', component: Backups },
    { path: '/updates', component: Updates },
    // Works while the panel is down, without a session check.
    { path: '/power/:action(reboot|poweroff)', component: Power, props: true, meta: { standalone: true } },
    { path: '/:rest(.*)', redirect: '/' },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.standalone) return true
  if (!session.loaded) await loadSession()
  if (to.meta.public) return session.loggedIn ? '/' : true
  if (!session.loggedIn) return '/login'
  if (session.mustChange && to.path !== '/password') return '/password'
  if (!session.mustChange && !session.setupDone && to.path !== '/setup') return '/setup'
  return true
})
