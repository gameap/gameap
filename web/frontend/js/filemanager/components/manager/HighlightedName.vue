<template>
    <template v-for="(segment, index) in segments" v-bind:key="index">
        <mark v-if="segment.hit" class="fm-name-match">{{ segment.text }}</mark>
        <template v-else>{{ segment.text }}</template>
    </template>
</template>

<script setup>
import { computed } from 'vue'
import { findMatchRanges } from '../../textFold.js'

const props = defineProps({
    text: { type: String, required: true },
    query: { type: String, default: '' },
})

const segments = computed(() => {
    const { text, query } = props
    if (query === '') {
        return [{ text, hit: false }]
    }

    const ranges = findMatchRanges(text, query)
    if (ranges.length === 0) {
        return [{ text, hit: false }]
    }

    const result = []
    let pos = 0

    ranges.forEach(({ start, end }) => {
        if (start > pos) {
            result.push({ text: text.slice(pos, start), hit: false })
        }
        result.push({ text: text.slice(start, end), hit: true })
        pos = end
    })

    if (pos < text.length) {
        result.push({ text: text.slice(pos), hit: false })
    }

    return result
})
</script>
