<script setup>
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

// The first Supla Cloud account, made together with installing Supla.
// Empty e-mail: no account.
const account = defineModel({ type: Object, required: true })
const { t } = useI18n()
const wanted = computed(() => account.value.email !== '')

// The form does not go through until both passwords are the same.
const repeat = ref(null)
watch(() => [account.value.password, account.value.repeat, repeat.value], () => {
  repeat.value?.setCustomValidity(account.value.password === account.value.repeat ? '' : t('setup.passwordsDiffer'))
})
</script>

<template>
  <fieldset class="account">
    <legend>{{ $t('setup.account') }}</legend>
    <p class="muted small">{{ $t('setup.accountHelp') }}</p>
    <div class="field">
      <label for="first-email">{{ $t('addon.email') }}</label>
      <input id="first-email" v-model.trim="account.email" type="email" autocomplete="off">
    </div>
    <template v-if="wanted">
      <div class="field">
        <label for="first-pass">{{ $t('addon.password') }}</label>
        <input id="first-pass" v-model="account.password" type="password" minlength="8" required autocomplete="new-password">
      </div>
      <div class="field">
        <label for="first-repeat">{{ $t('setup.repeatPassword') }}</label>
        <input id="first-repeat" ref="repeat" v-model="account.repeat" type="password" required autocomplete="new-password">
      </div>
    </template>
  </fieldset>
</template>
