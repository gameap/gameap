<template>
  <n-alert type="warning" data-testid="idempotency-pending">
    <p>{{ trans('servers.request_pending') }}</p>
    <div class="flex flex-wrap items-center gap-3 mt-3">
      <a :href="checkUrl" target="_blank" rel="noopener" class="underline">
        {{ trans('servers.request_check_state') }}
      </a>
      <button type="button" class="underline" data-testid="idempotency-reset" @click="$emit('reset')">
        {{ trans('servers.request_start_new') }}
      </button>
    </div>
  </n-alert>
</template>

<script setup>
import {NAlert} from 'naive-ui'
import {trans} from '@/i18n/i18n'

defineProps({checkUrl: {type: String, required: true}})
defineEmits(['reset'])
</script>
