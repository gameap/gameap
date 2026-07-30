<template>
    <div>
        <div v-if="selectedItems.length === 0" class="text-red-500 dark:text-red-400">
            {{ lang.modal.chmod.noSelected }}
        </div>
        <div v-else>
            <div class="mb-3">
                <div v-if="selectedItems.length === 1">
                    <strong class="break-all">{{ selectedItems[0].basename }}</strong>
                </div>
                <div v-else>
                    <strong>{{ selectedItems.length }} {{ lang.modal.chmod.itemsSelected }}</strong>
                    <span v-if="mixedSource" class="ml-2 text-orange-500 dark:text-orange-400">
                        ({{ lang.modal.chmod.mixedSource }})
                    </span>
                </div>
            </div>

            <table class="fm-chmod-table w-full mb-3">
                <thead>
                    <tr>
                        <th></th>
                        <th class="text-center">{{ lang.modal.chmod.read }}</th>
                        <th class="text-center">{{ lang.modal.chmod.write }}</th>
                        <th class="text-center">{{ lang.modal.chmod.execute }}</th>
                    </tr>
                </thead>
                <tbody>
                    <tr v-for="row in rows" :key="row.role">
                        <td>{{ lang.modal.chmod[row.role] }}</td>
                        <td class="text-center">
                            <n-checkbox v-model:checked="permissions[row.r]" @update:checked="syncOctal" />
                        </td>
                        <td class="text-center">
                            <n-checkbox v-model:checked="permissions[row.w]" @update:checked="syncOctal" />
                        </td>
                        <td class="text-center">
                            <n-checkbox v-model:checked="permissions[row.x]" @update:checked="syncOctal" />
                        </td>
                    </tr>
                </tbody>
            </table>

            <div class="mb-2">
                <label for="fm-input-chmod-octal" class="block mb-1">{{ lang.modal.chmod.octalLabel }}</label>
                <n-input
                    id="fm-input-chmod-octal"
                    v-model:value="octalText"
                    :status="invalidMode ? 'error' : undefined"
                    :maxlength="3"
                    @update:value="syncCheckboxes"
                />
                <div v-if="invalidMode" class="text-red-500 text-sm mt-1">
                    {{ lang.modal.chmod.invalidMode }}
                </div>
            </div>

            <div class="text-sm text-stone-500 dark:text-stone-400">
                {{ lang.modal.chmod.preview }}: <code>{{ symbolicString }}</code>
            </div>
        </div>
    </div>
</template>

<script setup>
/* eslint-disable no-bitwise */
import { ref, computed, reactive, onMounted } from 'vue'
import { NCheckbox, NInput } from 'naive-ui'
import { useFileManagerStore } from '../../../stores/useFileManagerStore.js'
import { useTranslate } from '../../../composables/useTranslate.js'
import { useModal } from '../../../composables/useModal.js'

const PERM_KEYS = ['ownerR', 'ownerW', 'ownerX', 'groupR', 'groupW', 'groupX', 'otherR', 'otherW', 'otherX']

const PERM_BITS = {
    ownerR: 0o400,
    ownerW: 0o200,
    ownerX: 0o100,
    groupR: 0o040,
    groupW: 0o020,
    groupX: 0o010,
    otherR: 0o004,
    otherW: 0o002,
    otherX: 0o001,
}

const DEFAULT_MODE = 0o644

const rows = [
    { role: 'owner', r: 'ownerR', w: 'ownerW', x: 'ownerX' },
    { role: 'group', r: 'groupR', w: 'groupW', x: 'groupX' },
    { role: 'other', r: 'otherR', w: 'otherW', x: 'otherX' },
]

function permissionsFromMode(mode) {
    const result = {}
    PERM_KEYS.forEach((key) => {
        result[key] = (mode & PERM_BITS[key]) !== 0
    })

    return result
}

function modeFromPermissions(perms) {
    let mode = 0
    PERM_KEYS.forEach((key) => {
        if (perms[key]) mode |= PERM_BITS[key]
    })

    return mode
}

function modeToOctalText(mode) {
    return (mode & 0o777).toString(8).padStart(3, '0')
}

function modeToSymbolic(mode) {
    const triplet = (bits) => [bits & 0o4 ? 'r' : '-', bits & 0o2 ? 'w' : '-', bits & 0o1 ? 'x' : '-'].join('')

    return triplet((mode >> 6) & 0o7) + triplet((mode >> 3) & 0o7) + triplet(mode & 0o7)
}

const fm = useFileManagerStore()
const { lang } = useTranslate()
const { hideModal } = useModal()

const permissions = reactive(permissionsFromMode(DEFAULT_MODE))
const octalText = ref(modeToOctalText(DEFAULT_MODE))
const invalidMode = ref(false)
const mixedSource = ref(false)

const selectedItems = computed(() => fm.selectedItems)
const modeNumber = computed(() => modeFromPermissions(permissions))
const symbolicString = computed(() => modeToSymbolic(modeNumber.value))
const submitDisabled = computed(() => invalidMode.value || selectedItems.value.length === 0)

function applyMode(mode) {
    const next = permissionsFromMode(mode)
    PERM_KEYS.forEach((key) => {
        permissions[key] = next[key]
    })
    octalText.value = modeToOctalText(mode)
    invalidMode.value = false
}

function syncOctal() {
    octalText.value = modeToOctalText(modeNumber.value)
    invalidMode.value = false
}

function syncCheckboxes(value) {
    const trimmed = (value ?? octalText.value).trim()
    if (!/^[0-7]{1,3}$/.test(trimmed)) {
        invalidMode.value = true

        return
    }

    const parsed = parseInt(trimmed, 8)
    if (Number.isNaN(parsed) || parsed < 0 || parsed > 0o777) {
        invalidMode.value = true

        return
    }

    const next = permissionsFromMode(parsed)
    PERM_KEYS.forEach((key) => {
        permissions[key] = next[key]
    })
    invalidMode.value = false
}

function submit() {
    if (submitDisabled.value) return

    const items = selectedItems.value.map((item) => ({ path: item.path }))

    fm.chmod({ items, mode: modeNumber.value }).then(() => {
        hideModal()
    })
}

onMounted(() => {
    if (selectedItems.value.length === 0) return

    const modes = selectedItems.value.map((item) => item.mode).filter((m) => typeof m === 'number')

    if (modes.length === 0) {
        applyMode(DEFAULT_MODE)

        return
    }

    const allSame = modes.every((m) => m === modes[0])
    if (allSame) {
        applyMode(modes[0])
    } else {
        mixedSource.value = true
        applyMode(DEFAULT_MODE)
    }
})

defineExpose({
    footerButtons: computed(() => [
        {
            label: lang.value.btn.submit,
            color: 'green',
            icon: 'lock',
            action: submit,
            disabled: submitDisabled.value,
        },
        { label: lang.value.btn.cancel, color: 'black', icon: 'close', action: hideModal },
    ]),
})
</script>

<style lang="scss">
.fm-chmod-table {
    th,
    td {
        padding: 0.4rem 0.75rem;
    }

    thead th {
        font-weight: 600;
        @apply border-b;
    }

    tbody tr:hover {
        @apply bg-stone-50 dark:bg-stone-800;
    }
}
</style>
