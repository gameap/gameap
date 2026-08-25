<template>
  <n-form-item
    :path="path"
    :rule="rule"
    :feedback="serverError || undefined"
    :validation-status="serverError ? 'error' : undefined"
    :data-testid="`server-setting-${definition.name}`"
  >
    <template #label>
      <span class="inline-flex items-center gap-2">
        {{ label }}
        <GStatusBadge v-if="definition.adminVar" color="light" :text="trans('servers.settings_admin_var')" />
      </span>
    </template>

    <div class="w-full">
      <VarValueField
        :definition="definition"
        :value="value"
        :disabled="disabled"
        :placeholder="placeholder"
        @update:value="$emit('update:value', $event)"
      />
      <!-- The description lives in the default slot on purpose: n-form-item
           renders `children || feedback` before the validation explains, so a
           #feedback slot would silently swallow every error message. -->
      <small v-if="description" class="block mt-1 text-stone-500 dark:text-stone-400">
        {{ description }}
      </small>
    </div>
  </n-form-item>
</template>

<script setup>
import { computed } from 'vue'
import { NFormItem } from 'naive-ui'
import { GStatusBadge } from '@gameap/ui'
import { trans, getCurrentLanguage } from '@/i18n/i18n'
import { buildVarRules, localizedDescription, localizedLabel } from '@/parts/gameModVars'
import VarValueField from '@/components/input/VarValueField.vue'

const props = defineProps({
  /** normalizeVarDefinition() output */
  definition: {
    type: Object,
    required: true,
  },
  value: {
    type: [String, Number, Boolean],
    default: null,
  },
  /** lodash path into the n-form model */
  path: {
    type: String,
    required: true,
  },
  /** One message from a 422 response, shown instead of the client rules. */
  serverError: {
    type: String,
    default: null,
  },
  disabled: {
    type: Boolean,
    default: false,
  },
})

defineEmits(['update:value'])

const locale = getCurrentLanguage()

const label = computed(() => localizedLabel(props.definition, locale) || props.definition.name)
const description = computed(() => localizedDescription(props.definition, locale))
const rule = computed(() => buildVarRules(props.definition, label.value))

const placeholder = computed(() => {
  if (props.definition.type === 'bool' || props.definition.default === null) {
    return ''
  }

  return String(props.definition.default)
})
</script>
