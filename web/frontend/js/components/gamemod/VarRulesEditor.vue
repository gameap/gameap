<template>
  <div class="space-y-2" data-testid="var-rules-editor">
    <div class="flex items-center gap-2">
      <n-switch
        :value="rules.required === true"
        size="small"
        @update:value="setRule('required', $event === true ? true : undefined)"
      />
      <span class="text-sm">{{ trans('games.var_rule_required') }}</span>
    </div>

    <div v-if="isNumeric" class="grid grid-cols-1 sm:grid-cols-2 gap-2">
      <n-form-item :label="trans('games.var_rule_min')" :show-feedback="false">
        <n-input-number
          :value="rules.min ?? null"
          :precision="type === 'int' ? 0 : undefined"
          size="small"
          class="w-full"
          clearable
          :status="rangeProblem ? 'error' : undefined"
          @update:value="setRule('min', $event)"
        />
      </n-form-item>
      <n-form-item :label="trans('games.var_rule_max')" :show-feedback="false">
        <n-input-number
          :value="rules.max ?? null"
          :precision="type === 'int' ? 0 : undefined"
          size="small"
          class="w-full"
          clearable
          :status="rangeProblem ? 'error' : undefined"
          @update:value="setRule('max', $event)"
        />
      </n-form-item>
    </div>

    <p v-if="rangeProblem" class="text-sm text-red-500">{{ rangeProblem }}</p>

    <template v-if="acceptsText">
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
        <n-form-item :label="trans('games.var_rule_min_length')" :show-feedback="false">
          <n-input-number
            :value="rules.min_length ?? null"
            :precision="0"
            :min="0"
            size="small"
            class="w-full"
            clearable
            :status="lengthRangeProblem ? 'error' : undefined"
            @update:value="setRule('min_length', $event)"
          />
        </n-form-item>
        <n-form-item :label="trans('games.var_rule_max_length')" :show-feedback="false">
          <n-input-number
            :value="rules.max_length ?? null"
            :precision="0"
            :min="1"
            size="small"
            class="w-full"
            clearable
            :status="lengthRangeProblem ? 'error' : undefined"
            @update:value="setRule('max_length', $event)"
          />
        </n-form-item>
      </div>

      <p v-if="lengthRangeProblem" class="text-sm text-red-500">{{ lengthRangeProblem }}</p>

      <n-form-item :label="trans('games.var_rule_pattern')" :show-feedback="false">
        <n-input
          :value="rules.pattern ?? ''"
          size="small"
          placeholder="[a-z0-9_]+"
          :status="patternProblem ? 'error' : undefined"
          @update:value="setRule('pattern', $event)"
        />
      </n-form-item>

      <p v-if="patternProblem" class="text-sm text-red-500">{{ patternProblem }}</p>

      <!-- A live tester catches the RE2-vs-JavaScript surprise here rather than
           in a 422 the server owner sees weeks later. -->
      <div v-if="rules.pattern" class="flex items-center gap-2">
        <n-input
          v-model:value="patternSample"
          size="small"
          :placeholder="trans('games.var_rule_pattern_test')"
          class="max-w-xs"
        />
        <n-tag v-if="patternSample" size="small" :type="patternMatches ? 'success' : 'error'">
          {{ patternMatches
            ? trans('games.var_rule_pattern_match')
            : trans('games.var_rule_pattern_no_match') }}
        </n-tag>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { NFormItem, NInput, NInputNumber, NSwitch, NTag } from 'naive-ui'
import { trans } from '@/i18n/i18n'
import {
  NUMERIC_TYPES,
  TEXTUAL_TYPES,
  VAR_PATTERN_MAX_LENGTH,
  compilePattern,
  isRe2Compatible,
} from '@/parts/gameModVars'

const props = defineProps({
  type: {
    type: String,
    default: 'string',
  },
  allowCustom: {
    type: Boolean,
    default: false,
  },
})

/** The raw rules object, or null when nothing is set. */
const model = defineModel({ type: Object, default: null })

const patternSample = ref('')

const rules = computed(() => model.value ?? {})

const isNumeric = computed(() => NUMERIC_TYPES.includes(props.type))
const acceptsText = computed(
  () => TEXTUAL_TYPES.includes(props.type) || (props.type === 'select' && props.allowCustom),
)

// The panel rejects a lower bound above its upper bound with a 422, so the two
// number pairs are checked here as well, the same way the pattern is.
const rangeProblem = computed(() => boundsProblem(rules.value.min, rules.value.max, 'games.var_rule_range_invalid'))

const lengthRangeProblem = computed(
  () => boundsProblem(rules.value.min_length, rules.value.max_length, 'games.var_rule_length_range_invalid'),
)

function boundsProblem(lower, upper, message) {
  if (typeof lower !== 'number' || typeof upper !== 'number') {
    return null
  }

  return lower > upper ? trans(message) : null
}

const patternProblem = computed(() => {
  const pattern = String(rules.value.pattern ?? '').trim()
  if (!pattern) {
    return null
  }

  if (pattern.length > VAR_PATTERN_MAX_LENGTH) {
    return trans('games.var_rule_pattern_too_long')
  }

  if (!compilePattern(pattern)) {
    return trans('games.var_rule_pattern_invalid')
  }

  if (!isRe2Compatible(pattern)) {
    return trans('games.var_rule_pattern_unsupported')
  }

  return null
})

const patternMatches = computed(() => {
  const compiled = compilePattern(String(rules.value.pattern ?? ''))

  return Boolean(compiled && compiled.test(patternSample.value))
})

function setRule(key, value) {
  const next = { ...rules.value }

  if (value === undefined || value === null || value === '') {
    delete next[key]
  } else {
    next[key] = value
  }

  // The schema requires a non-empty rules object, so an emptied one goes away.
  model.value = Object.keys(next).length > 0 ? next : null
}
</script>
