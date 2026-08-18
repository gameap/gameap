<template>
    <div>
        <div v-if="errors.length">
            <n-list>
                <n-list-item v-for="(item, index) in errors" :key="index">
                    <n-thing>
                        <template #header>
                            <n-text type="error">{{ item.status }}</n-text>
                        </template>
                        {{ item.message }}
                    </n-thing>
                </n-list-item>
            </n-list>
        </div>
        <div v-else>
            <n-text type="success">{{ lang.modal.status.noErrors }}</n-text>
        </div>
    </div>
</template>

<script setup>
import { computed } from 'vue'
import { useMessagesStore } from '../../../stores/useMessagesStore.js'
import { useTranslate } from '../../../composables/useTranslate.js'
import { useModal } from '../../../composables/useModal.js'

const messages = useMessagesStore()
const { lang } = useTranslate()
const { hideModal } = useModal()

const errors = computed(() => messages.errors)

function clearErrors() {
    messages.clearErrors()
}

defineExpose({
    footerButtons: computed(() => [
        { label: lang.value.btn.cancel, color: 'black', icon: 'close', action: hideModal },
        { label: lang.value.btn.clear, color: 'red', icon: 'clear', action: clearErrors, disabled: !errors.value.length },
    ]),
})
</script>
