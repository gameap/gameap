<template>
  <section class="dark:bg-stone-800">
    <div class="flex flex-col items-center justify-center px-6 py-8 mx-auto my-10 lg:py-0">

      <div class="w-full bg-white rounded shadow dark:border md:mt-0 sm:max-w-md xl:p-0 dark:bg-stone-800 dark:border-stone-700">
        <div class="w-full bg-stone-700 p-8 rounded-t">
          <h1 class="text-xl font-bold leading-tight tracking-tight text-white md:text-2xl dark:text-white">
            {{ twoFactorRequired ? trans('two_factor.verify_title') : trans('auth.sign_in') }}
          </h1>
        </div>

        <template v-if="!twoFactorRequired">
          <div class="p-6 space-y-4 md:space-y-6 sm:p-8">
            <form class="space-y-4 md:space-y-6" action="#">
              <div>
                <label for="email" class="block mb-2 text-sm font-medium text-stone-900 dark:text-white">{{ trans('auth.username_email') }}</label>
                <input type="email" name="email" id="email" v-model="email" class="bg-stone-50 border border-stone-300 text-stone-900 sm:text-sm rounded block w-full p-2.5 dark:bg-stone-700 dark:border-stone-600 dark:placeholder-stone-400 dark:text-white dark:focus:border-info" required="">
              </div>
              <div>
                <label for="password" class="block mb-2 text-sm font-medium text-stone-900 dark:text-white">{{ trans('auth.password') }}</label>
                <input type="password" name="password" id="password" v-model="password" @keyup.enter="login" class="bg-stone-50 border border-stone-300 text-stone-900 sm:text-sm rounded block w-full p-2.5 dark:bg-stone-700 dark:border-stone-600 dark:placeholder-stone-400 dark:text-white dark:focus:border-info" required="">
              </div>
              <div class="flex items-center justify-between">
                <div class="flex items-start">
                  <div class="flex items-center h-5">
                    <input v-model="remember" id="remember" aria-describedby="remember" type="checkbox" class="w-4 h-4 border border-stone-300 rounded bg-stone-50 dark:bg-stone-700 dark:border-stone-600" required="">
                  </div>
                  <div class="ml-3 text-sm">
                    <label for="remember" class="text-stone-500 dark:text-stone-300">{{ trans('auth.remember') }}</label>
                  </div>
                </div>
              </div>
              <CaptchaWidget
                  v-if="captcha.provider"
                  ref="captchaRef"
                  :provider="captcha.provider"
                  :site-key="captcha.site_key"
              />
            </form>
          </div>

          <div class="w-full bg-stone-700 p-8 rounded-b">
            <GButton color="white" :loading="submitting" v-on:click="login">
              <GIcon v-if="!submitting" name="sign-in" class="mr-1" />
              <span>{{ submitting ? trans('main.wait') : trans('auth.sign_in') }}</span>
            </GButton>
          </div>
        </template>

        <TwoFactorVerifyForm v-else class="p-6 sm:p-8" :loading="submitting" @verify="onVerify" />
      </div>
    </div>
  </section>
</template>

<script setup>
import {onMounted, ref} from "vue"
import {useRouter} from "vue-router"
import { GIcon } from "@gameap/ui"
import {trans} from "../i18n/i18n"
import axios from "../config/axios"
import {useAuthStore} from "../store/auth"
import {errorNotification} from "../parts/dialogs";
import GButton from "../components/GButton.vue";
import TwoFactorVerifyForm from "./forms/TwoFactorVerifyForm.vue";
import CaptchaWidget from "./forms/CaptchaWidget.vue";

const authStore = useAuthStore()
const router = useRouter()

const email = ref(null)
const password = ref(null)
const remember = ref(false)
const twoFactorRequired = ref(false)
const captcha = ref({provider: null, site_key: null})
const captchaRef = ref(null)

onMounted(async () => {
  try {
    const {data} = await axios.get('/api/config/public')
    if (data && data.captcha) {
      captcha.value = data.captcha
    }
  } catch (e) {
    // A missing/unavailable public config must not block the login form;
    // captcha simply stays disabled client-side.
    console.error('Failed to load public config:', e)
  }
})

const showError = (error) => {
  if (error.response) {
    if ('data' in error.response && 'message' in error.response.data) {
      errorNotification(error.response.data.message)
    } else {
      errorNotification(error)
    }
  } else {
    errorNotification(error)
  }
}

// Finishes a successful auth flow with an in-app navigation instead of a full
// page reload. The login response carries only a minimal user object (no
// roles), so the full profile is re-fetched first — the reload used to do
// this via initializeAuth() — otherwise role-gated UI (admin sidebar, profile
// page) renders against an incomplete profile. Navigating next lets the route
// settle to the dashboard within the same microtask checkpoint as the
// auth-state change, so the authenticated layout never paints over the
// still-mounted login form. Plugins are loaded afterwards (the reload used to
// trigger this in app.js). Errors here must not surface as login failures, so
// each step swallows its own error.
const completeLogin = async () => {
  try {
    await authStore.fetchProfile()
  } catch (error) {
    console.error('Post-login profile fetch failed:', error)
  }

  try {
    await router.push({ name: 'home' })
  } catch (error) {
    console.error('Post-login navigation failed:', error)
  }

  try {
    const { loadPlugins } = await import('../plugins/loader')
    await loadPlugins(router)
  } catch (error) {
    console.error('Failed to load plugins after login:', error)
  }
}

// Covers the whole flow: the captcha token, the login request and the
// post-login navigation are all part of the wait the button has to show.
// Cleared in a finally, so a rejected attempt — or the 2FA step taking over —
// hands the form back; after a successful login the view is already gone.
const submitting = ref(false)

const login = async () => {
  if (submitting.value) {
    return
  }

  submitting.value = true

  try {
    let captchaToken = ""

    if (captcha.value.provider) {
      captchaToken = await captchaRef.value.getToken()
      if (!captchaToken) {
        errorNotification(trans('auth.captcha_required'))

        return
      }
    }

    const result = await authStore.login({
      login: email.value,
      password: password.value,
      remember: remember.value ? "on" : null,
      captcha: captchaToken,
    })

    if (result && result.twoFactorRequired) {
      twoFactorRequired.value = true

      return
    }

    await completeLogin()
  } catch (error) {
    if (captchaRef.value) {
      captchaRef.value.reset()
    }
    showError(error)
  } finally {
    submitting.value = false
  }
}

const onVerify = async (code) => {
  if (submitting.value) {
    return
  }

  submitting.value = true

  try {
    await authStore.verifyTwoFactor(code)
    await completeLogin()
  } catch (error) {
    showError(error)
  } finally {
    submitting.value = false
  }
}
</script>
