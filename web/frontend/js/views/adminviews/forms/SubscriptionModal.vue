<template>
  <GModal
      v-model:show="showModal"
      :title="trans('plugins.subscription_required')"
      style="width: 500px; max-width: 90vw;"
  >
    <div class="space-y-4">
      <p class="text-stone-600 dark:text-stone-400">
        {{ trans('plugins.subscription_info') }}
      </p>

      <div v-if="plugin" class="flex items-center gap-3 p-3 bg-stone-50 dark:bg-stone-800 rounded-lg">
        <PluginIcon :plugin="plugin" img-class="w-12 h-12 rounded" fallback-class="text-4xl text-stone-400" />
        <div>
          <div class="font-medium">{{ plugin.name }}</div>
          <div class="text-sm text-stone-500">{{ plugin.summary }}</div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <GButton color="gray" @click="showModal = false">
          {{ trans('main.close') }}
        </GButton>
        <GButton color="orange" @click="onGetSubscription">
          <GIcon name="external-link" class="mr-1" />
          {{ trans('plugins.get_subscription') }}
        </GButton>
      </div>
    </template>
  </GModal>
</template>

<script setup>
import { computed } from 'vue'
import { trans } from '@/i18n/i18n'
import { GIcon, GModal } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import PluginIcon from '@/components/plugins/PluginIcon.vue'

const props = defineProps({
  show: {
    type: Boolean,
    default: false
  },
  plugin: {
    type: Object,
    default: null
  }
})

const emit = defineEmits(['update:show', 'get-subscription'])

const showModal = computed({
  get: () => props.show,
  set: (value) => emit('update:show', value)
})

function onGetSubscription() {
  if (props.plugin?.subscription_url) {
    window.open(props.plugin.subscription_url, '_blank')
  }
  emit('get-subscription')
  showModal.value = false
}
</script>
