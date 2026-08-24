<template>
    <template v-for="(segment, index) in segments" v-bind:key="index">
        <mark v-if="segment.hit" class="fm-name-match">{{ segment.text }}</mark>
        <template v-else>{{ segment.text }}</template>
    </template>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
    text: { type: String, required: true },
    query: { type: String, default: '' },
})

const segments = computed(() => {
    const { text, query } = props
    const needle = query.toLowerCase()
    if (!needle) {
        return [{ text, hit: false }]
    }

    const haystack = text.toLowerCase()
    const result = []
    let pos = 0
    let idx = haystack.indexOf(needle, pos)

    while (idx !== -1) {
        if (idx > pos) {
            result.push({ text: text.slice(pos, idx), hit: false })
        }
        result.push({ text: text.slice(idx, idx + needle.length), hit: true })
        pos = idx + needle.length
        idx = haystack.indexOf(needle, pos)
    }

    if (pos < text.length) {
        result.push({ text: text.slice(pos), hit: false })
    }

    return result.length ? result : [{ text: '', hit: false }]
})
</script>
