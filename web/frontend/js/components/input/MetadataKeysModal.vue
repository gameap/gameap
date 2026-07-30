<template>
  <n-modal
      :show="show"
      :on-update:show="(v) => $emit('update:show', v)"
      preset="card"
      :title="trans('metadata_keys.title')"
      :bordered="false"
      :segmented="{ content: 'soft', footer: 'soft' }"
      style="width: 900px; max-width: 90vw;"
  >
    <div class="overflow-y-auto pr-1 max-h-[70vh]">
      <div v-for="group in groups" :key="group.name" class="mb-5 last:mb-0">
        <h5 class="text-base font-semibold text-stone-900 dark:text-white">
          {{ trans(`metadata_keys.group_${group.name}`) }}
        </h5>
        <p class="mb-2 text-sm text-stone-500 dark:text-stone-400">
          {{ trans(`metadata_keys.group_${group.name}_hint`) }}
        </p>

        <table class="stone-table">
          <thead class="stone-table-header">
          <tr>
            <th class="px-2 py-2 w-1/3">{{ trans('labels.key') }}</th>
            <th class="px-2 py-2">{{ trans('labels.description') }}</th>
            <th class="px-2 py-2 whitespace-nowrap">{{ trans('main.actions') }}</th>
          </tr>
          </thead>

          <tbody>
          <tr v-for="item in group.keys" :key="item.key" class="stone-table-row">
            <td class="px-2 py-2 w-1/3 align-top">
              <span class="bg-stone-100 dark:bg-stone-600 highlighter-rouge p-1 rounded break-all">
                {{ item.key }}
              </span>
            </td>
            <td class="px-2 py-2 align-top">
              <div>{{ trans(`metadata_keys.${item.key}`) }}</div>
              <div class="mt-1 text-xs text-stone-500 dark:text-stone-400 whitespace-pre-line break-all">
                {{ trans('metadata_keys.example') }}: {{ item.example }}
              </div>
            </td>
            <td class="px-2 py-2 align-top">
              <GButton color="green" size="small" @click="$emit('select', item.key)">
                <GIcon name="paste" class="mr-0.5" />
                <span class="hidden lg:inline">{{ trans('metadata_keys.insert') }}</span>
              </GButton>
            </td>
          </tr>
          </tbody>
        </table>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <GButton color="black" @click="$emit('update:show', false)">
          <GIcon name="close" class="mr-1" />
          {{ trans('main.close') }}
        </GButton>
      </div>
    </template>
  </n-modal>
</template>

<script setup>
import { NModal } from "naive-ui"
import { GIcon } from '@gameap/ui'
import GButton from "../GButton.vue"
import { trans } from "@/i18n/i18n"

defineProps({
  show: {type: Boolean, default: false},
  groups: {type: Array, default: () => []},
})

defineEmits(['update:show', 'select'])
</script>
