<template>
  <section class="py-16 px-4">
    <div class="mx-auto max-w-screen-sm text-center">
      <template v-if="failed">
        <p class="mb-4 text-lg text-stone-900 dark:text-white">{{ trans('auth.sso_failed') }}</p>
        <GButton size="large" color="black" :route="{name: 'login'}">
          {{ trans('auth.sign_in') }}
        </GButton>
      </template>
      <div v-else-if="twoFactorRequired" class="mx-auto max-w-sm text-left">
        <h1 class="mb-4 text-center text-xl font-bold text-stone-900 dark:text-white">
          {{ trans('two_factor.verify_title') }}
        </h1>
        <TwoFactorVerifyForm :loading="verifying" @verify="onVerify" />
      </div>
      <p v-else class="text-lg text-stone-600 dark:text-stone-300">
        {{ trans('auth.sso_signing_in') }}
      </p>
    </div>
  </section>
</template>

<script setup>
// Lands the browser after an external system (a billing panel) handed the
// customer a single-use sign-in link. The ticket arrives in the URL FRAGMENT,
// never the query string: a fragment is not sent to the server, so it stays
// out of proxy access logs, and it is not carried in the Referer header.
import {onMounted, ref} from "vue";
import {useRouter} from "vue-router";
import {trans} from "@/i18n/i18n";
import GButton from "@/components/GButton.vue";
import TwoFactorVerifyForm from "@/views/forms/TwoFactorVerifyForm.vue";
import {useAuthStore} from "@/store/auth";

const router = useRouter()
const authStore = useAuthStore()

const failed = ref(false)

// An account with a second factor still has to prove it — single sign-on is
// not a way around 2FA. The challenge is redeemed on this same view so the
// panel-requested redirect (held in pendingRedirect) survives the extra step,
// rather than being dropped on the way to a separate login screen.
const twoFactorRequired = ref(false)
const verifying = ref(false)
const pendingRedirect = ref('')

const readTicket = () => {
  const fragment = window.location.hash.replace(/^#/, '')

  if (!fragment) {
    return ''
  }

  return new URLSearchParams(fragment).get('t') || ''
}

// Only same-origin paths are followed. The server already validated the
// redirect when the ticket was minted, but the value travels back through the
// response, so it is checked again here rather than trusted twice.
const safeRedirect = (path) => {
  if (!path || !path.startsWith('/') || path.startsWith('//') || path.startsWith('/\\')) {
    return '/'
  }

  return path
}

// finishSso runs the shared tail of a successful exchange, whether or not a
// second factor was involved: refresh the full profile (the exchange/verify
// response carries only a minimal user), navigate to the validated redirect,
// then load plugins. Each step swallows its own error so a post-auth hiccup
// never looks like a sign-in failure.
const finishSso = async (redirect) => {
  try {
    await authStore.fetchProfile()
  } catch (error) {
    console.error('SSO profile fetch failed:', error)
  }

  await router.replace(safeRedirect(redirect))

  try {
    const {loadPlugins} = await import('../plugins/loader')
    await loadPlugins(router)
  } catch (error) {
    console.error('Failed to load plugins after SSO:', error)
  }
}

const onVerify = async (code) => {
  if (verifying.value) {
    return
  }

  verifying.value = true

  try {
    await authStore.verifyTwoFactor(code)
    await finishSso(pendingRedirect.value)
  } catch (error) {
    console.error('SSO two-factor verification failed:', error)
    failed.value = true
    twoFactorRequired.value = false
  } finally {
    verifying.value = false
  }
}

onMounted(async () => {
  const ticket = readTicket()

  // Scrub the ticket from the address bar before anything can await, so it
  // does not linger in browser history or get copied out of the URL.
  window.history.replaceState(null, '', '/sso')

  if (!ticket) {
    failed.value = true

    return
  }

  let result

  try {
    result = await authStore.consumeSsoTicket(ticket)
  } catch (error) {
    console.error('SSO exchange failed:', error)
    failed.value = true

    return
  }

  if (result.twoFactorRequired) {
    pendingRedirect.value = result.redirectTo || ''
    twoFactorRequired.value = true

    return
  }

  await finishSso(result.redirectTo)
})
</script>
