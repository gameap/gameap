<template>
  <a v-if="link" :class='classes' :href="link" :disabled="disabled">
    <slot></slot>
  </a>
  <router-link v-else-if="route" :to="route" :class='classes' :disabled="disabled">
    <slot></slot>
  </router-link>
  <button v-else :class='classes' v-on:click="buttonClick" :disabled="disabled">
    <slot></slot>
  </button>
</template>

<script setup>
import {computed} from 'vue'

const defaultClass = 'inline-block align-middle text-center select-none ' +
    'font-normal whitespace-no-wrap rounded leading-normal no-underline'

const defaultDisabledClass = 'cursor-not-allowed'

const colors = {
  black: 'bg-stone-700 text-white hover:bg-stone-800 dark:bg-stone-900 dark:hover:bg-stone-950',
  white: 'text-black bg-white hover:bg-stone-100 dark:bg-stone-800 dark:text-white dark:border-stone-600 dark:hover:bg-stone-700 dark:hover:border-stone-600',
  green: 'bg-lime-500 text-white hover:bg-lime-600 dark:bg-lime-800 dark:hover:bg-lime-900 dark:text-stone-200',
  red: 'bg-red-500 text-white hover:bg-red-600 dark:bg-red-800 dark:hover:bg-red-900 dark:text-stone-200',
  orange: 'bg-orange-400 text-white hover:bg-orange-500 dark:bg-orange-800 dark:hover:bg-orange-900 dark:text-stone-200',
  blue: 'bg-sky-500 text-white hover:bg-sky-600 dark:bg-sky-800 dark:hover:bg-sky-900 dark:text-stone-200',
}

const disabledColors = {
  black: 'bg-stone-600 text-stone-400 dark:bg-stone-900 dark:text-stone-600',
  white: 'bg-stone-300 text-stone-400',
  green: 'bg-lime-400 text-lime-200 dark:bg-lime-900 dark:text-lime-950',
  red: 'bg-red-400 text-red-200 dark:bg-red-900 dark:text-red-950',
  orange: 'bg-orange-300 text-orange-200 dark:bg-orange-900 dark:text-orange-950',
  blue: 'bg-sky-400 text-sky-200 dark:bg-sky-900 dark:text-sky-950',
}

const sizes = {
  small: 'text-xs py-1.5 px-2',
  middle: 'py-2 px-3',
  large: 'text-lg py-3 px-4',
}

const props = defineProps({
  color: 'white',
  size: 'middle',
  link: null,
  route: null,
  class: '',
  disabled: false,
})

const classes = computed(() => {
  const color = props.disabled
      ? (disabledColors[props.color] || disabledColors.white)
      : (colors[props.color] || colors.white)

  const size = sizes[props.size] || sizes.middle

  let c = []

  c.push(defaultClass)

  if (props.disabled) {
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