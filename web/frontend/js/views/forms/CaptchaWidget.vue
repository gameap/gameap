<template>
  <div v-if="isWidget" ref="containerRef" class="mt-2"></div>
</template>

<script setup>
import { onMounted, ref } from "vue"

const props = defineProps({
  provider: { type: String, required: true },
  siteKey: { type: String, required: true },
})

const containerRef = ref(null)
const token = ref("")
let widgetId = null

// recaptcha_v2 and turnstile render a visible challenge and report the
// solution through a callback; recaptcha_v3 is invisible and produces a
// fresh token on demand at submit time.
const isWidget = props.provider === "recaptcha_v2" || props.provider === "turnstile"

const scriptCache = {}

const loadScript = (src) => {
  if (scriptCache[src]) {
    return scriptCache[src]
  }

  scriptCache[src] = new Promise((resolve, reject) => {
    const el = document.createElement("script")
    el.src = src
    el.async = true
    el.defer = true
    el.onload = () => resolve()
    el.onerror = () => reject(new Error(`failed to load ${src}`))
    document.head.appendChild(el)
  })

  return scriptCache[src]
}

const waitFor = (predicate, timeout = 10000) =>
  new Promise((resolve, reject) => {
    const started = Date.now()
    const tick = () => {
      if (predicate()) {
        resolve()
      } else if (Date.now() - started > timeout) {
        reject(new Error("captcha script timed out"))
      } else {
        setTimeout(tick, 50)
      }
    }
    tick()
  })

const renderRecaptchaV2 = async () => {
  await loadScript("https://www.google.com/recaptcha/api.js?render=explicit")
  await waitFor(() => window.grecaptcha && window.grecaptcha.render)

  widgetId = window.grecaptcha.render(containerRef.value, {
    sitekey: props.siteKey,
    callback: (t) => {
      token.value = t
    },
    "expired-callback": () => {
      token.value = ""
    },
    "error-callback": () => {
      token.value = ""
    },
  })
}

const renderTurnstile = async () => {
  await loadScript("https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit")
  await waitFor(() => window.turnstile && window.turnstile.render)

  widgetId = window.turnstile.render(containerRef.value, {
    sitekey: props.siteKey,
    callback: (t) => {
      token.value = t
    },
    "expired-callback": () => {
      token.value = ""
    },
    "error-callback": () => {
      token.value = ""
    },
  })
}

const loadRecaptchaV3 = async () => {
  await loadScript(`https://www.google.com/recaptcha/api.js?render=${props.siteKey}`)
  await waitFor(() => window.grecaptcha && window.grecaptcha.execute)
}

onMounted(() => {
  if (props.provider === "recaptcha_v2") {
    renderRecaptchaV2().catch((e) => console.error(e))
  } else if (props.provider === "turnstile") {
    renderTurnstile().catch((e) => console.error(e))
  } else if (props.provider === "recaptcha_v3") {
    loadRecaptchaV3().catch((e) => console.error(e))
  }
})

// getToken returns the solution token to send with the login request. For
// the invisible reCAPTCHA v3 it executes a fresh challenge each call;
// for the widget providers it returns the token captured by the callback
// ("" until the visitor solves it).
const getToken = async () => {
  if (props.provider === "recaptcha_v3") {
    await waitFor(() => window.grecaptcha && window.grecaptcha.execute)

    return window.grecaptcha.execute(props.siteKey, { action: "login" })
  }

  return token.value
}

// reset re-arms the widget after a rejected login so the visitor can solve
// a fresh challenge.
const reset = () => {
  token.value = ""
  if (widgetId === null) {
    return
  }
  if (props.provider === "recaptcha_v2" && window.grecaptcha) {
    window.grecaptcha.reset(widgetId)
  } else if (props.provider === "turnstile" && window.turnstile) {
    window.turnstile.reset(widgetId)
  }
}

defineExpose({ getToken, reset })
</script>
