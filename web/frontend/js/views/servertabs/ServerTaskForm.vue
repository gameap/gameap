<script setup>
import { computed, ref, watch } from 'vue'
import dayjs from 'dayjs'
import {
    NCollapse,
    NCollapseItem,
    NDatePicker,
    NForm,
    NFormItem,
    NInput,
    NInputNumber,
    NRadio,
    NRadioGroup,
    NSelect,
    NSpace,
} from 'naive-ui'
import { GModal, GSwitch } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import { useServerTasksStore } from '@/store/serverTasks'
import { errorNotification } from '@/parts/dialogs'
import { pluralize, trans } from '@/i18n/i18n'

const props = defineProps({
    show: { type: Boolean, default: false },
    task: { type: Object, default: null },
    taskIndex: { type: Number, default: null },
    privileges: {
        type: Object,
        default: () => ({ start: true, stop: true, restart: true, update: true }),
    },
})

const emit = defineEmits(['update:show', 'saved'])

const tasksStore = useServerTasksStore()

const REPEAT_ENDLESSLY = 0
const REPEAT_ONCE = 1
const RADIO_CUSTOM = 'custom'
const RADIO_ENDLESS = 'endless'
const RADIO_ONCE = 'once'

const form = ref(createBlankForm())
const repeatMode = ref(RADIO_ONCE)
const repeatCount = ref(1)
const repeatPeriodValue = ref(1)
const repeatPeriodUnit = ref('hours')
const errors = ref({})
const submitting = ref(false)

function createBlankForm() {
    return {
        name: '',
        command: '',
        enabled: true,
        timezone: '',
        execute_date: dayjs().add(1, 'hour').format('YYYY-MM-DD HH:mm:ss'),
        overlap_policy: 'skip',
        catchup_policy: 'skip',
        payload: null,
    }
}

const isEdit = computed(() => props.taskIndex !== null && props.taskIndex !== undefined)
const title = computed(() =>
    isEdit.value ? trans('servers_tasks.edit_task') : trans('servers_tasks.new_task'),
)
const submitLabel = computed(() =>
    isEdit.value ? trans('main.save') : trans('main.create'),
)

const commandOptions = computed(() => {
    const opts = []
    if (props.privileges.restart) opts.push({ label: trans('servers.restart'), value: 'restart' })
    if (props.privileges.start) opts.push({ label: trans('servers.start'), value: 'start' })
    if (props.privileges.stop) opts.push({ label: trans('servers.stop'), value: 'stop' })
    if (props.privileges.update) opts.push({ label: trans('servers.update'), value: 'update' })
    if (props.privileges.update) opts.push({ label: trans('servers.reinstall'), value: 'reinstall' })

    return opts
})

const unitOptions = computed(() => [
    { label: pluralize('minute', parseInt(repeatPeriodValue.value || 0, 10)), value: 'minutes' },
    { label: pluralize('hour', parseInt(repeatPeriodValue.value || 0, 10)), value: 'hours' },
    { label: pluralize('day', parseInt(repeatPeriodValue.value || 0, 10)), value: 'days' },
    { label: pluralize('week', parseInt(repeatPeriodValue.value || 0, 10)), value: 'weeks' },
    { label: pluralize('month', parseInt(repeatPeriodValue.value || 0, 10)), value: 'months' },
    { label: pluralize('year', parseInt(repeatPeriodValue.value || 0, 10)), value: 'years' },
])

const timezoneOptions = computed(() => {
    const list = (typeof Intl !== 'undefined' && typeof Intl.supportedValuesOf === 'function')
        ? Intl.supportedValuesOf('timeZone')
        : []

    return list.map((tz) => ({ label: tz, value: tz }))
})

const repeatPeriodDisabled = computed(() => repeatMode.value === RADIO_ONCE)
const repeatCountDisabled = computed(() => repeatMode.value !== RADIO_CUSTOM)
const minPeriodValue = computed(() => (repeatPeriodUnit.value === 'minutes' ? 10 : 1))

function loadTaskIntoForm(task) {
    form.value = createBlankForm()

    if (!task) {
        repeatMode.value = RADIO_ONCE
        repeatCount.value = 1
        repeatPeriodValue.value = 1
        repeatPeriodUnit.value = 'hours'

        return
    }

    form.value.name = task.name ?? ''
    form.value.command = task.command ?? ''
    form.value.enabled = task.enabled !== false
    form.value.timezone = task.timezone ?? ''
    form.value.execute_date = task.execute_date
        ? dayjs(task.execute_date).format('YYYY-MM-DD HH:mm:ss')
        : dayjs().add(1, 'hour').format('YYYY-MM-DD HH:mm:ss')
    form.value.overlap_policy = task.overlap_policy || 'skip'
    form.value.catchup_policy = task.catchup_policy || 'skip'
    form.value.payload = task.payload ?? null

    const repeat = parseInt(task.repeat, 10)
    if (repeat === REPEAT_ENDLESSLY) {
        repeatMode.value = RADIO_ENDLESS
        repeatCount.value = 1
    } else if (repeat === REPEAT_ONCE) {
        repeatMode.value = RADIO_ONCE
        repeatCount.value = 1
    } else {
        repeatMode.value = RADIO_CUSTOM
        repeatCount.value = repeat
    }

    if (task.repeat_period) {
        const [rawValue, rawUnit] = String(task.repeat_period).split(' ')
        const parsedValue = parseInt(rawValue, 10)
        if (!Number.isNaN(parsedValue) && parsedValue > 0) {
            repeatPeriodValue.value = parsedValue
        }
        const normalizedUnit = normalizeUnitPlural(rawUnit)
        if (normalizedUnit) {
            repeatPeriodUnit.value = normalizedUnit
        }
    }
}

function normalizeUnitPlural(unit) {
    if (!unit) return null
    switch (unit) {
        case 'm':
        case 'min':
        case 'mins':
        case 'minute':
        case 'minutes':
            return 'minutes'
        case 'h':
        case 'hr':
        case 'hour':
        case 'hours':
            return 'hours'
        case 'd':
        case 'day':
        case 'days':
            return 'days'
        case 'w':
        case 'week':
        case 'weeks':
            return 'weeks'
        case 'month':
        case 'months':
            return 'months'
        case 'y':
        case 'year':
        case 'years':
            return 'years'
        default:
            return null
    }
}

watch(
    () => props.show,
    (open) => {
        if (open) {
            loadTaskIntoForm(props.task)
            errors.value = {}
        }
    },
    { immediate: true },
)

function computeRepeat() {
    if (repeatMode.value === RADIO_ONCE) return REPEAT_ONCE
    if (repeatMode.value === RADIO_ENDLESS) return REPEAT_ENDLESSLY

    return Math.max(1, Math.min(255, parseInt(repeatCount.value, 10) || 1))
}

function validate() {
    const next = {}

    if (!form.value.command) {
        next.command = trans('servers_tasks.empty_command')
    }
    if (!form.value.execute_date) {
        next.execute_date = trans('servers_tasks.empty_task_date')
    } else if (dayjs(form.value.execute_date).isBefore(dayjs().subtract(1, 'minute'))) {
        next.execute_date = trans('servers_tasks.empty_task_date')
    }
    if (form.value.name && form.value.name.length > 128) {
        next.name = trans('servers_tasks.name_too_long')
    }
    if (repeatMode.value === RADIO_CUSTOM) {
        const value = parseInt(repeatCount.value, 10)
        if (Number.isNaN(value) || value < 1 || value > 255) {
            next.repeat_count = trans('servers_tasks.invalid_repeat_value')
        }
    }
    if (repeatMode.value !== RADIO_ONCE) {
        if (!repeatPeriodUnit.value) {
            next.repeat_period = trans('servers_tasks.empty_period_unit')
        } else if (!repeatPeriodValue.value || repeatPeriodValue.value < 1) {
            next.repeat_period = trans('servers_tasks.empty_period')
        } else if (repeatPeriodUnit.value === 'minutes' && repeatPeriodValue.value < 10) {
            next.repeat_period = trans('servers_tasks.minimum_period')
        }
    }

    errors.value = next

    return Object.keys(next).length === 0
}

function buildPayload() {
    const repeat = computeRepeat()
    const payload = {
        command: form.value.command,
        execute_date: form.value.execute_date,
        repeat,
        enabled: form.value.enabled,
        overlap_policy: form.value.overlap_policy,
        catchup_policy: form.value.catchup_policy,
    }

    if (form.value.name && form.value.name.trim().length > 0) {
        payload.name = form.value.name.trim()
    } else {
        payload.name = null
    }

    if (form.value.timezone && form.value.timezone.trim().length > 0) {
        payload.timezone = form.value.timezone.trim()
    } else {
        payload.timezone = null
    }

    if (repeat !== REPEAT_ONCE) {
        payload.repeat_period = `${repeatPeriodValue.value} ${repeatPeriodUnit.value}`
    } else {
        payload.repeat_period = ''
    }

    if (form.value.payload != null) {
        payload.payload = form.value.payload
    }

    return payload
}

async function submit() {
    if (!validate() || submitting.value) {
        return
    }

    submitting.value = true
    try {
        const payload = buildPayload()
        if (isEdit.value) {
            await tasksStore.updateTask(props.taskIndex, payload)
        } else {
            await tasksStore.storeTask(payload)
        }
        emit('saved')
        close()
    } catch (e) {
        errorNotification(e)
    } finally {
        submitting.value = false
    }
}

function close() {
    emit('update:show', false)
}
</script>

<template>
    <GModal
        :show="show"
        :title="title"
        preset="card"
        :bordered="false"
        :style="{ width: '640px', maxWidth: '95vw' }"
        @update:show="(value) => emit('update:show', value)"
    >
        <NForm label-placement="top" require-mark-placement="right-hanging" size="medium">
            <NFormItem
                :label="trans('servers_tasks.name')"
                :feedback="errors.name"
                :validation-status="errors.name ? 'error' : undefined"
            >
                <NInput
                    v-model:value="form.name"
                    :placeholder="trans('servers_tasks.name_placeholder')"
                    :maxlength="128"
                    show-count
                    clearable
                />
            </NFormItem>

            <div class="grid md:grid-cols-2 gap-4">
                <NFormItem
                    :label="trans('servers_tasks.command')"
                    :feedback="errors.command"
                    :validation-status="errors.command ? 'error' : undefined"
                >
                    <NSelect v-model:value="form.command" :options="commandOptions" />
                </NFormItem>

                <NFormItem :label="trans('servers_tasks.enabled')">
                    <div class="flex items-center gap-3">
                        <GSwitch v-model:value="form.enabled" />
                        <span
                            class="text-sm"
                            :class="form.enabled
                                ? 'text-lime-700 dark:text-lime-300'
                                : 'text-stone-500 dark:text-stone-400'"
                        >
                            {{ form.enabled
                                ? trans('servers_tasks.active')
                                : trans('servers_tasks.paused') }}
                        </span>
                    </div>
                </NFormItem>
            </div>

            <div class="grid md:grid-cols-2 gap-4">
                <NFormItem
                    :label="trans('servers_tasks.date')"
                    :feedback="errors.execute_date"
                    :validation-status="errors.execute_date ? 'error' : undefined"
                >
                    <NDatePicker
                        v-model:formatted-value="form.execute_date"
                        value-format="yyyy-MM-dd HH:mm:ss"
                        type="datetime"
                        clearable
                        class="w-full"
                    />
                </NFormItem>

                <NFormItem :label="trans('servers_tasks.timezone')">
                    <NSelect
                        v-if="timezoneOptions.length > 0"
                        v-model:value="form.timezone"
                        :options="timezoneOptions"
                        filterable
                        clearable
                        :placeholder="trans('servers_tasks.timezone_placeholder')"
                    />
                    <NInput
                        v-else
                        v-model:value="form.timezone"
                        :placeholder="trans('servers_tasks.timezone_placeholder')"
                        clearable
                    />
                </NFormItem>
            </div>

            <NFormItem :label="trans('servers_tasks.repeat')">
                <NRadioGroup v-model:value="repeatMode">
                    <NSpace vertical>
                        <NRadio :value="RADIO_ONCE">{{ trans('servers_tasks.no_repeat') }}</NRadio>
                        <NRadio :value="RADIO_ENDLESS">{{ trans('servers_tasks.endlessly_repeat') }}</NRadio>
                        <NRadio :value="RADIO_CUSTOM">{{ trans('servers_tasks.custom_repeat') }}</NRadio>
                    </NSpace>
                </NRadioGroup>
            </NFormItem>

            <div class="grid md:grid-cols-2 gap-4">
                <NFormItem
                    :label="trans('servers_tasks.repeat_num')"
                    :feedback="errors.repeat_count"
                    :validation-status="errors.repeat_count ? 'error' : undefined"
                >
                    <NInputNumber
                        v-model:value="repeatCount"
                        :disabled="repeatCountDisabled"
                        :min="1"
                        :max="255"
                        class="w-full"
                    />
                </NFormItem>

                <NFormItem
                    :label="trans('servers_tasks.repeat_period')"
                    :feedback="errors.repeat_period"
                    :validation-status="errors.repeat_period ? 'error' : undefined"
                >
                    <div class="flex gap-2 w-full">
                        <NInputNumber
                            v-model:value="repeatPeriodValue"
                            :disabled="repeatPeriodDisabled"
                            :min="minPeriodValue"
                            class="flex-1"
                        />
                        <NSelect
                            v-model:value="repeatPeriodUnit"
                            :disabled="repeatPeriodDisabled"
                            :options="unitOptions"
                            class="flex-1"
                        />
                    </div>
                </NFormItem>
            </div>

            <NCollapse class="mt-2">
                <NCollapseItem :title="trans('servers_tasks.advanced')" name="advanced">
                    <div class="grid md:grid-cols-2 gap-4">
                        <NFormItem :label="trans('servers_tasks.overlap_policy')">
                            <NRadioGroup v-model:value="form.overlap_policy">
                                <NSpace vertical>
                                    <div>
                                        <NRadio value="skip">
                                            {{ trans('servers_tasks.overlap_skip') }}
                                        </NRadio>
                                        <p class="text-xs text-stone-500 dark:text-stone-400 ml-6 mt-1">
                                            {{ trans('servers_tasks.overlap_skip_hint') }}
                                        </p>
                                    </div>
                                    <div>
                                        <NRadio value="queue">
                                            {{ trans('servers_tasks.overlap_queue') }}
                                        </NRadio>
                                        <p class="text-xs text-stone-500 dark:text-stone-400 ml-6 mt-1">
                                            {{ trans('servers_tasks.overlap_queue_hint') }}
                                        </p>
                                    </div>
                                </NSpace>
                            </NRadioGroup>
                        </NFormItem>

                        <NFormItem :label="trans('servers_tasks.catchup_policy')">
                            <NRadioGroup v-model:value="form.catchup_policy">
                                <NSpace vertical>
                                    <div>
                                        <NRadio value="skip">
                                            {{ trans('servers_tasks.catchup_skip') }}
                                        </NRadio>
                                        <p class="text-xs text-stone-500 dark:text-stone-400 ml-6 mt-1">
                                            {{ trans('servers_tasks.catchup_skip_hint') }}
                                        </p>
                                    </div>
                                    <div>
                                        <NRadio value="run_once">
                                            {{ trans('servers_tasks.catchup_run_once') }}
                                        </NRadio>
                                        <p class="text-xs text-stone-500 dark:text-stone-400 ml-6 mt-1">
                                            {{ trans('servers_tasks.catchup_run_once_hint') }}
                                        </p>
                                    </div>
                                </NSpace>
                            </NRadioGroup>
                        </NFormItem>
                    </div>
                </NCollapseItem>
            </NCollapse>
        </NForm>

        <template #footer>
            <div class="flex justify-end gap-2">
                <GButton color="white" @click="close">
                    {{ trans('main.close') }}
                </GButton>
                <GButton color="blue" :disabled="submitting" @click="submit">
                    {{ submitLabel }}
                </GButton>
            </div>
        </template>
    </GModal>
</template>
