<template>
    <div class="fm-content d-flex flex-column">
        <bread-crumb v-bind:manager="manager" />
        <div class="fm-content-body h-[40vh]">
            <table-view v-if="viewType === 'table'" v-bind:manager="manager" />
            <grid-view v-else v-bind:manager="manager" />
        </div>
    </div>
</template>

<script>
// Components
import DiskList from './DiskList.vue';
import BreadCrumb from './BreadCrumb.vue';
import TableView from './TableView.vue';
import GridView from './GridView.vue';

export default {
    name: 'Manager',
    components: {
        DiskList,
        BreadCrumb,
        TableView,
        GridView,
    },
    props: {
        manager: { type: String, required: true },
    },
    computed: {
        /**
         * view type - grid or table
         * @returns {any}
         */
        viewType() {
            return this.$store.state.fm[this.manager].viewType;
        },
    },
};
</script>

<style lang="scss">
.fm-content {
    padding-left: 1rem;

    .fm-content-body {
        overflow-y: scroll;

        //@media screen and (min-height: 800px) {
        //    height: 370px;
        //}
        //@media screen and (max-height: 800px) {
        //    height: 270px;
        //}
        //@media screen and (max-height: 600px) {
        //    height: 170px;
        //}
    }
}
</style>
