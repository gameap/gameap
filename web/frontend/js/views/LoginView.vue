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
                <input type="email" name="email" id="email" v-model="email" class="bg-stone-50 border border-stone-300 text-stone-900 sm:text-sm rounded focus:ring-primary-600 focus:border-primary-600 block w-full p-2.5 dark:bg-stone-700 dark:border-stone-600 dark:placeholder-stone-400 dark:text-white dark:focus:ring-blue-500 dark:focus:border-blue-500" required="">
              </div>
              <div>
                <label for="password" class="block mb-2 text-sm font-medium text-stone-900 dark:text-white">{{ trans('auth.password') }}</label>
                <input type="password" name="password" id="password" v-model="password" @keyup.enter="login" class="bg-stone-50 border border-stone-300 text-stone-900 sm:text-sm rounded focus:ring-primary-600 focus:border-primary-600 block w-full p-2.5 dark:bg-stone-700 dark:border-stone-600 dark:placeholder-stone-400 dark:text-white dark:focus:ring-blue-500 dark:focus:border-blue-500" required="">
              </div>
              <div class="flex items-center justify-between">
                <div class="flex items-start">
                  <div class="flex items-center h-5">
                    <input v-model="remember" id="remember" aria-describedby="remember" type="checkbox" class="w-4 h-4 border border-stone-300 rounded bg-stone-50 focus:ring-3 focus:ring-primary-300 dark:bg-stone-700 dark:border-stone-600 dark:focus:ring-primary-600 dark:ring-offset-stone-800" required="">
                  </div>
                  <div class="ml-3 text-sm">
                    <label for="remember" class="text-stone-500 dark:text-stone-300">{{ trans('auth.remember') }}</label>
                  </div>
                </div>
              </div>
            </form>
          </div>

          <div class="w-full bg-stone-700 p-8 rounded-b">
            <button @click="login" type="button" class="text-stone-900 bg-white border focus:outline-none hover:bg-stone-100 focus:ring-4 focus:ring-stone-100 font-medium rounded text-sm px-5 py-2.5 me-2 mb-2 dark:bg-stone-800 dark:text-white dark:border-stone-600 dark:hover:bg-stone-700 dark:hover:border-stone-600 dark:focus:ring-stone-700">
              <GIcon name="sign-in" class="mr-1" />
              <span>{{ trans('auth.sign_in')}}</span>
            </button>
          </div>
        </template>

        <TwoFactorVerifyForm v-else class="p-6 sm:p-8" @verify="onVerify" />
      </div>
    </div>
  </section>
</template>

<script setup>
import {ref} from "vue"
import { GIcon } from "@gameap/ui"
import {trans} from "../i18n/i18n"
import {useAuthStore} from "../store/auth"
import {errorNotification} from "../parts/dialogs";
import TwoFactorVerifyForm from "./forms/TwoFactorVerifyForm.vue";

const authStore = useAuthStore()

const email = ref(null)
const password = ref(null)
const remember = ref(false)
const twoFactorRequired = ref(false)

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

const login = () => {
  authStore.login({
    login: email.value,
    password: password.value,
    remember: remember.value ? "on" : null,
  }).then((result) => {
    if (result && result.twoFactorRequired) {
      twoFactorRequired.value = true

      return
    }

    location.reload()
  }).catch(showError)
}

const onVerify = (code) => {
  authStore.verifyTwoFactor(code).then(() => {
    location.reload()
  }).catch(showError)
}
</script>
