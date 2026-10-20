import { createRouter, createWebHistory } from 'vue-router'
import { session, loadSession } from './api'
import Login from './views/Login.vue'
import Password from './views/Password.vue'
import Setup from './views/Setup.vue'
import Dashboard from './views/Dashboard.vue'
import Addons from './views/Addons.vue'
import Addon from './views/Addon.vue'
import System from './views/System.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: Login, meta: { public: true } },
    { path: '/password', component: Password },
    { path: '/setup', component: Setup },
    { path: '/', component: Dashboard },
    { path: '/addons', component: Addons },
    { path: '/addons/:id', component: Addon, props: true },
    { path: '/system', component: System },
    { path: '/:rest(.*)', redirect: '/' },
  ],
})

router.beforeEach(async (to) => {
  if (!session.loaded) await loadSession()
  if (to.meta.public) return session.loggedIn ? '/' : true
  if (!session.loggedIn) return '/login'
  if (session.mustChange && to.path !== '/password') return '/password'
  if (!session.mustChange && !session.setupDone && to.path !== '/setup') return '/setup'
  return true
})
