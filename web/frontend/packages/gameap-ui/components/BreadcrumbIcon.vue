<template>
  <GIcon v-if="isRegistered" :name="icon" class="align-middle mr-0.5" />
  <i v-else-if="icon" :class="[icon, 'align-middle mr-0.5']"></i>
</template>

<script setup>
import { computed } from 'vue'
import GIcon from './GIcon.vue'
import { hasIcon } from '../icons/registry.js'

const props = defineProps({
  // Either a registry icon name or, for backwards compatibility, raw CSS classes.
  icon: {
    type: String,
    default: '',
  },
})

const isRegistered = computed(() => Boolean(props.icon) && hasIcon(props.icon))
</script>
