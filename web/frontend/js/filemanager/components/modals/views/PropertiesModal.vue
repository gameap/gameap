<template>
    <div>
        <div class="grid grid-cols-[auto_minmax(0,1fr)_auto] gap-3 my-3 hover:bg-surface-hover rounded p-1">
            <div><strong>{{ lang.modal.properties.disk }}:</strong></div>
            <div>{{ selectedDisk }}</div>
            <div v-if="canCopy" class="text-right cursor-pointer">
                <GIcon
                    @click="copyToClipboard(selectedDisk)"
                    :title="lang.clipboard.copy"
                    name="copy"
                />
            </div>
        </div>
        <div class="grid grid-cols-[auto_minmax(0,1fr)_auto] gap-3 my-3 hover:bg-surface-hover rounded p-1">
            <div><strong>{{ lang.modal.properties.name }}:</strong></div>
            <div class="break-all">{{ selectedItem.basename }}</div>
            <div v-if="canCopy" class="text-right cursor-pointer">
                <GIcon
                    @click="copyToClipboard(selectedItem.basename)"
                    :title="lang.clipboard.copy"
                    name="copy"
                />
            </div>
        </div>
        <div class="grid grid-cols-[auto_minmax(0,1fr)_auto] gap-3 my-3 hover:bg-surface-hover rounded p-1">
            <div><strong>{{ lang.modal.properties.path }}:</strong></div>
            <div class="break-all">{{ selectedItem.path }}</div>
            <div v-if="canCopy" class="text-right cursor-pointer">
                <GIcon
                    @click="copyToClipboard(selectedItem.path)"
                    :title="lang.clipboard.copy"
                    name="copy"
                />
            </div>
        </div>
        <template v-if="selectedItem.type === 'file'">
            <div class="grid grid-cols-[auto_minmax(0,1fr)_auto] gap-3 my-3 hover:bg-surface-hover rounded p-1">
                <div><strong>{{ lang.modal.properties.size }}:</strong></div>
                <div>{{ bytesToHuman(selectedItem.size) }}</div>
                <div v-if="canCopy" class="text-right cursor-pointer">
                    <GIcon
                        @click="copyToClipboard(bytesToHuman(selectedItem.size))"
                        :title="lang.clipboard.copy"
                        name="copy"
                    />
                </div>
            </div>
        </template>
        <template v-if="selectedItem.hasOwnProperty('timestamp')">
            <div class="grid grid-cols-[auto_minmax(0,1fr)_auto] gap-3 my-3 hover:bg-surface-hover rounded p-1">
                <div><strong>{{ lang.modal.properties.modified }}:</strong></div>
                <div>{{ timestampToDate(selectedItem.timestamp) }}</div>
                <div v-if="canCopy" class="text-right cursor-pointer">
                    <GIcon
                        @click="copyToClipboard(timestampToDate(selectedItem.timestamp))"
                        :title="lang.clipboard.copy"
                        name="copy"
                    />
                </div>
            </div>
        </template>
        <template v-if="selectedItem.hasOwnProperty('acl')">
            <div class="grid grid-cols-[auto_minmax(0,1fr)_auto] gap-3 my-3 p-1">
                <div>{{ lang.modal.properties.access }}:</div>
                <div>{{ lang.modal.properties['access_' + selectedItem.acl] }}</div>
            </div>
        </template>
        <template v-if="hasMode">
            <div class="grid grid-cols-[auto_minmax(0,1fr)_auto] gap-3 my-3 hover:bg-surface-hover rounded p-1 items-center">
                <div><strong>{{ lang.modal.properties.permissions }}:</strong></div>
                <div>
                    <code>{{ symbolicMode }}</code>
                    <span class="ml-2 text-muted">({{ octalMode }})</span>
                </div>
                <div class="text-right">
                    <GButton color="black" size="small" @click="openChmod">
                        <GIcon name="lock" class="mr-1" />
                        {{ lang.btn.edit }}
                    </GButton>
                </div>
            </div>
        </template>
    </div>
</template>

<script setup>
/* eslint-disable no-bitwise */
import { computed } from 'vue'
import { GIcon } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import { useFileManagerStore } from '../../../stores/useFileManagerStore.js'
import { useModalStore } from '../../../stores/useModalStore.js'
import { useTranslate } from '../../../composables/useTranslate.js'
import { useHelper } from '../../../composables/useHelper.js'
import { useModal } from '../../../composables/useModal.js'
import { notification } from '@/parts/dialogs.js'
import { copyToClipboard as copyText, isClipboardSupported } from '@/utils/clipboard.js'

const fm = useFileManagerStore()
const modal = useModalStore()
const { lang } = useTranslate()
const { bytesToHuman, timestampToDate } = useHelper()
const { hideModal } = useModal()

const selectedDisk = computed(() => fm.selectedDisk)
const selectedItem = computed(() => fm.selectedItems[0])
const canCopy = isClipboardSupported()

const hasMode = computed(() => selectedItem.value && typeof selectedItem.value.mode === 'number')
const octalMode = computed(() => (selectedItem.value.mode & 0o777).toString(8).padStart(3, '0'))
const symbolicMode = computed(() => {
    const { mode } = selectedItem.value
    const triplet = (bits) => [bits & 0o4 ? 'r' : '-', bits & 0o2 ? 'w' : '-', bits & 0o1 ? 'x' : '-'].join('')

    return triplet((mode >> 6) & 0o7) + triplet((mode >> 3) & 0o7) + triplet(mode & 0o7)
})

function openChmod() {
    modal.setModalState({ modalName: 'ChmodModal', show: true })
}

async function copyToClipboard(text) {
    const ok = await copyText(text)
    if (!ok) {
        console.error('Failed to copy to clipboard')

        return
    }

    notification({
        content: lang.value.notifications.copyToClipboard,
        type: 'success',
    })
}

defineExpose({
    footerButtons: computed(() => [
        { label: lang.value.btn.cancel, color: 'black', icon: 'close', action: hideModal },
    ]),
})
</script>
