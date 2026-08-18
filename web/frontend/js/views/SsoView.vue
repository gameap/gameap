<template>
  <section class="py-16 px-4">
    <div class="mx-auto max-w-screen-sm text-center">
      <template v-if="failed">
        <p class="mb-4 text-lg text-stone-900 dark:text-white">{{ trans('auth.sso_failed') }}</p>
        <GButton size="large" color="black" :route="{name: 'login'}">
          {{ trans('auth.sign_in') }}
        </GButton>
      </template>
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
import {useAuthStore} from "@/store/auth";

const router = useRouter()
const authStore = useAuthStore()

const failed = ref(false)

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

  // An account with a second factor still has to prove it — single sign-on
  // is not a way around 2FA. The login view picks the challenge up from the
  // store and renders the code form.
  if (result.twoFactorRequired) {
    await router.replace({name: 'login'})

    return
  }

  try {
    await authStore.fetchProfile()
  } catch (error) {
    console.error('SSO profile fetch failed:', error)
  }

  await router.replace(safeRedirect(result.redirectTo))

  try {
    const {loadPlugins} = await import('../plugins/loader')
    await loadPlugins(router)
  } catch (error) {
    console.error('Failed to load plugins after SSO:', error)
  }
})
</script>
