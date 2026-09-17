<template>
  <GModal
      v-model:show="show"
      :title="trans('hub.title')"
      style="width: 600px; max-width: 94vw"
  >
    <p class="mb-4">{{ trans('hub.description') }}</p>

    <ol class="mb-4 space-y-2">
      <li v-for="(step, index) in steps" :key="index" class="flex items-center gap-3">
        <span class="flex items-center justify-center shrink-0 w-6 h-6 rounded-full bg-stone-100 dark:bg-stone-700">
          {{ index + 1 }}
        </span>
        <span>{{ step }}</span>
      </li>
    </ol>

    <div class="space-y-1 text-muted">
      <p class="flex items-start gap-2">
        <GIcon name="check" class="mt-1" />
        <span>{{ trans('hub.moderation') }}</span>
      </p>
      <p class="flex items-start gap-2">
        <GIcon name="upload" class="mt-1" />
        <span>{{ trans('hub.publish') }}</span>
      </p>
    </div>

    <template #footer>
      <div class="flex flex-wrap gap-2">
        <GButton color="black" :link="url" target="_blank">
          <GIcon name="external-link" class="mr-1" />
          {{ trans('hub.open') }}
        </GButton>
        <GButton color="black" :route="{name: 'admin.games.import'}">
          <GIcon name="download" class="mr-1" />
          {{ trans('games.import') }}
        </GButton>
      </div>
    </template>
  </GModal>
</template>

<script setup>
import {GIcon, GModal} from "@gameap/ui"
import {trans} from "@/i18n/i18n"
import GButton from "@/components/GButton.vue"
import {hubHost, hubUrl} from "@/parts/hub"

const show = defineModel('show', {type: Boolean, default: false})

const url = hubUrl()

const steps = [
  trans('hub.step_find', {host: hubHost()}),
  trans('hub.step_download'),
  trans('hub.step_import'),
]
</script>
