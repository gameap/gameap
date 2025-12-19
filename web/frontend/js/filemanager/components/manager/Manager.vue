<template>
    <div class="fm-content">
        <bread-crumb v-bind:manager="manager" />
        <div class="fm-content-body">
            <table-view v-bind:manager="manager" />
        </div>
    </div>
</template>

<script setup>
import BreadCrumb from './BreadCrumb.vue'
import TableView from './TableView.vue'

const props = defineProps({
    manager: { type: String, required: true },
})
</script>

<style lang="scss">
.fm-content {
    display: flex;
    flex-direction: column;
    height: 100%;

    .fm-content-body {
        flex: 1 1 0;
        min-height: 0;
        overflow-y: auto;
    }
}
</style>
