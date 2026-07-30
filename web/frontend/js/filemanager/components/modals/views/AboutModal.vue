<template>
    <div>
        <strong>{{ lang.modal.about.name }}</strong>
        <GDivider />
        <n-descriptions :column="1" label-placement="left">
            <n-descriptions-item :label="lang.modal.about.version">
                {{ version }}
            </n-descriptions-item>
            <n-descriptions-item :label="lang.modal.about.developer">
                Aleksandr Manekin (alexusmai@gmail.com)
            </n-descriptions-item>
            <n-descriptions-item :label="lang.modal.about.developer">
                Nikita Kuznetsov (nikita.hldm@gmail.com)
            </n-descriptions-item>
            <n-descriptions-item label="GitHub">
                <a class="font-medium text-info hover:underline" href="https://github.com/gameap/gameap-file-manager" target="_blank">
                    gameap/gameap-file-manager
                </a>
                <br />
                <a class="font-medium text-info hover:underline" href="https://github.com/gameap/vue-gameap-file-manager" target="_blank">
                    gameap/vue-laravel-file-manager
                </a>
            </n-descriptions-item>
        </n-descriptions>
    </div>
</template>

<script setup>
import { computed } from 'vue'
import { GDivider } from '@gameap/ui'
import { useSettingsStore } from '../../../stores/useSettingsStore.js'
import { useTranslate } from '../../../composables/useTranslate.js'
import { useModal } from '../../../composables/useModal.js'

const settings = useSettingsStore()
const { lang } = useTranslate()
const { hideModal } = useModal()

const version = computed(() => settings.version)

defineExpose({
    footerButtons: computed(() => [
        { label: lang.value.btn.cancel, color: 'black', icon: 'close', action: hideModal },
    ]),
})
</script>
