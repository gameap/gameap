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
import { providePluginContext } from '@/plugins'
import { filterSlotComponents } from '@/plugins/permissions'

const props = defineProps({
    name: {
        type: String,
        required: true
    },
    context: {
        type: Object,
        default: () => ({})
    },
    // Opt-in evaluation of the components' `checkPermission` / `checkGame`
    // conditions. Hosts that know the server context pass `{ abilities, game }`
    // here; without it every registered component is rendered.
    checkContext: {
        type: Object,
        default: null
    }
})

providePluginContext()

const pluginsStore = usePluginsStore()

const sortedComponents = computed(() => {
    const components = props.checkContext
        ? filterSlotComponents(pluginsStore.getSlotComponents(props.name), props.checkContext)
        : pluginsStore.getSlotComponents(props.name)

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
