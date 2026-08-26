<template>
  <a v-if="link" :class='classes' :href="link" :disabled="isDisabled">
    <GIcon v-if="loading" name="loading" class="mr-1" />
    <slot></slot>
  </a>
  <router-link v-else-if="route" :to="route" :class='classes' :disabled="isDisabled">
    <GIcon v-if="loading" name="loading" class="mr-1" />
    <slot></slot>
  </router-link>
  <button v-else :class='classes' v-on:click="buttonClick" :disabled="isDisabled">
    <GIcon v-if="loading" name="loading" class="mr-1" />
    <slot></slot>
  </button>
</template>

<script setup>
import {computed} from 'vue'
import {GIcon} from '@gameap/ui'

const defaultClass = 'inline-block align-middle text-center select-none border ' +
    'font-normal whitespace-nowrap rounded leading-normal no-underline'

const defaultDisabledClass = 'cursor-not-allowed'

// Every variant carries a border so the sizes match; only white shows it.
const colors = {
  black: 'bg-stone-700 text-white border-transparent hover:bg-stone-800 dark:bg-stone-900 dark:hover:bg-stone-950',
  white: 'text-black bg-white border-strong hover:bg-stone-100 dark:bg-stone-800 dark:text-white dark:hover:bg-stone-700',
  green: 'bg-primary text-white border-transparent hover:bg-primary-hover dark:bg-lime-800 dark:hover:bg-lime-900 dark:text-stone-200',
  red: 'bg-danger text-white border-transparent hover:bg-danger-hover dark:bg-red-800 dark:hover:bg-red-900 dark:text-stone-200',
  orange: 'bg-warning text-white border-transparent hover:bg-warning-hover dark:bg-orange-800 dark:hover:bg-orange-900 dark:text-stone-200',
  blue: 'bg-info text-white border-transparent hover:bg-info-hover dark:bg-sky-800 dark:hover:bg-sky-900 dark:text-stone-200',
}

const disabledColors = {
  black: 'bg-stone-600 text-stone-400 border-transparent dark:bg-stone-900 dark:text-stone-600',
  white: 'bg-stone-300 text-stone-400 border-strong dark:bg-stone-800 dark:text-stone-600',
  green: 'bg-lime-300 text-lime-100 border-transparent dark:bg-lime-900 dark:text-lime-950',
  red: 'bg-red-400 text-red-200 border-transparent dark:bg-red-900 dark:text-red-950',
  orange: 'bg-orange-300 text-orange-200 border-transparent dark:bg-orange-900 dark:text-orange-950',
  blue: 'bg-sky-400 text-sky-200 border-transparent dark:bg-sky-900 dark:text-sky-950',
}

const sizes = {
  small: 'text-xs py-1.5 px-2',
  middle: 'py-2 px-3',
  large: 'text-lg py-3 px-4',
}

const props = defineProps({
  color: { type: String, default: 'white' },
  size: { type: String, default: 'middle' },
  link: { type: String, default: null },
  route: { type: [String, Object], default: null },
  class: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  loading: { type: Boolean, default: false },
})

// A pending action must not be triggered twice, so loading implies disabled.
const isDisabled = computed(() => props.disabled || props.loading)

const classes = computed(() => {
  const color = isDisabled.value
      ? (disabledColors[props.color] || disabledColors.white)
      : (colors[props.color] || colors.white)

  const size = sizes[props.size] || sizes.middle

  let c = []

  c.push(defaultClass)

  if (isDisabled.value) {
    c.push(defaultDisabledClass)
  }

  c.push(color)
  c.push(size)

  if (props.class) {
    c.push(props.class)
  }

  return c.join(' ')
})

const emits = defineEmits(["click"])

const buttonClick = () => {
  emits("click")
}

</script>

<style scoped>

</style>