<template>
  <svg
    v-if="isSvgData"
    :viewBox="resolvedIcon.viewBox"
    :class="props.class"
    :style="svgStyle"
    fill="currentColor"
    aria-hidden="true"
    focusable="false"
  >
    <g :transform="resolvedIcon.transform">
      <path v-for="(path, index) in resolvedIcon.paths" :key="index" :d="path" />
    </g>
  </svg>
  <component
    v-else-if="isComponent"
    :is="resolvedIcon"
    :class="props.class"
    :style="sizeStyle"
  />
  <i
    v-else
    :class="[resolvedIcon, sizeClass, props.class]"
  />
</template>

<script setup>
import { computed } from 'vue'
import { getIcon, hasIcon } from '../icons/registry.js'
import { isSvgIcon } from '../icons/svgData.js'
import { isLegacyFontAwesomeClass, warnLegacyFontAwesomeIcon } from '../icons/legacy.js'

const props = defineProps({
  name: {
    type: String,
    required: true
  },
  size: {
    type: String,
    default: 'md',
    validator: (value) => ['sm', 'md', 'lg', 'xl'].includes(value)
  },
  class: {
    type: String,
    default: ''
  }
})

const resolvedIcon = computed(() => {
  if (hasIcon(props.name)) {
    return getIcon(props.name)
  }

  if (isLegacyFontAwesomeClass(props.name)) {
    warnLegacyFontAwesomeIcon(props.name)

    return props.name
  }

  console.warn(`GIcon: Unknown icon "${props.name}"`)

  return 'fa-solid fa-circle-question'
})

const isSvgData = computed(() => isSvgIcon(resolvedIcon.value))

const isComponent = computed(() => {
  return typeof resolvedIcon.value === 'object' || typeof resolvedIcon.value === 'function'
})

const fontAwesomeSizeMap = {
  sm: 'fa-sm',
  md: '',
  lg: 'fa-lg',
  xl: 'fa-2x'
}

const componentSizeMap = {
  sm: '1em',
  md: '1.25em',
  lg: '1.5em',
  xl: '2.5em'
}

const sizeClass = computed(() => fontAwesomeSizeMap[props.size] || '')

const sizeStyle = computed(() => ({
  width: componentSizeMap[props.size],
  height: componentSizeMap[props.size],
  display: 'inline-block',
  verticalAlign: '-0.25em'
}))

// Width follows the viewBox ratio so that non-square logos are not squashed.
const svgStyle = computed(() => {
  const height = componentSizeMap[props.size] || componentSizeMap.md

  return {
    width: `calc(${height} * ${resolvedIcon.value.aspect})`,
    height,
    display: 'inline-block',
    verticalAlign: '-0.25em'
  }
})
</script>
