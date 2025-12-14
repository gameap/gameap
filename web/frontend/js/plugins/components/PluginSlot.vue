<template>
    <component
        v-for="(item, index) in sortedComponents"
        :key="`${item.pluginId}-${index}`"
        :is="item.component"
        v-bind="mergedProps(item)"
    />
</template>

<script setup>
import { computed } from 'vue'
import { usePluginsStore } from '@/store/plugins'
import { providePluginContext } from '../context'

const props = defineProps({
    name: {
        type: String,
        required: true
    },
    context: {
        type: Object,
        default: () => ({})
    }
})

providePluginContext()

const pluginsStore = usePluginsStore()

const sortedComponents = computed(() => {
    const components = pluginsStore.getSlotComponents(props.name)
    return [...components].sort((a, b) => a.order - b.order)
})

function mergedProps(item) {
    return {
        ...item.props,
        ...props.context,
        pluginId: item.pluginId
    }
}
</script>
