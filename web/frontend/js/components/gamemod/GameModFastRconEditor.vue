<template>
  <div class="space-y-2" data-testid="mod-fastrcon-editor">
    <div v-for="(item, index) in items" :key="index" class="space-y-1">
      <div class="grid grid-cols-1 sm:grid-cols-[1fr_1fr_2rem_2rem] gap-2 sm:items-center">
        <n-input
          :value="item.info ?? ''"
          size="small"
          :maxlength="128"
          :placeholder="trans('games.info')"
          @update:value="setField(index, 'info', $event)"
        />
        <n-input
          :value="item.command ?? ''"
          size="small"
          :placeholder="trans('games.rcon_command')"
          @update:value="setField(index, 'command', $event)"
        />
        <n-button
          quaternary
          size="small"
          :type="item.i18n ? 'primary' : 'default'"
          :title="trans('games.translations')"
          @click="toggleTranslations(index)"
        >
          <GIcon name="languages" />
        </n-button>
        <n-button quaternary size="small" type="error" @click="removeItem(index)">
          <GIcon name="trash" />
        </n-button>
      </div>

      <div v-if="expanded.has(index)" class="pl-2 border-l-2 border-stone-200 dark:border-stone-700">
        <I18nEditor
          :fields="i18nFields"
          :model-value="item.i18n ?? null"
          @update:model-value="setField(index, 'i18n', $event)"
        />
      </div>
    </div>

    <n-button size="small" dashed @click="addItem">
      <GIcon name="add" class="mr-1" />
      {{ trans('main.add') }}
    </n-button>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { NButton, NInput } from 'naive-ui'
import { GIcon } from '@gameap/ui'
import { trans } from '@/i18n/i18n'
import I18nEditor from '@/components/gamemod/I18nEditor.vue'

/** The raw fast RCON rows, matching the API shape. */
const items = defineModel({ type: Array, default: () => [] })

const expanded = ref(new Set())

const i18nFields = [{ key: 'info', label: trans('games.info'), maxlength: 128 }]

function setField(index, key, value) {
  const next = [...items.value]
  next[index] = { ...next[index], [key]: value }
  items.value = next
}

function addItem() {
  items.value = [...items.value, { info: '', command: '' }]
}

function removeItem(index) {
  const next = [...items.value]
  next.splice(index, 1)
  items.value = next
  expanded.value = new Set()
}

function toggleTranslations(index) {
  const next = new Set(expanded.value)

  if (next.has(index)) {
    next.delete(index)
  } else {
    next.add(index)
  }

  expanded.value = next
}
</script>
