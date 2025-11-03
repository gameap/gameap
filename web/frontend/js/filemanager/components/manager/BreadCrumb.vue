<template>
    <div class="fm-breadcrumb">
        <nav aria-label="breadcrumb" class="flex px-3 py-1.5 rounded text-stone-700 border border-stone-200 bg-stone-50 dark:bg-stone-800 dark:border-stone-700">
            <ol class="inline-flex items-center space-x-1 md:space-x-2 rtl:space-x-reverse" v-bind:class="[manager === activeManager ? 'active-manager' : 'bg-light']">
                <li class="breadcrumb-item" v-on:click="selectMainDirectory">
                    <span class="badge bg-secondary dark:text-stone-400">
                        <i class="fa-solid fa-hard-drive"></i>
                    </span>
                </li>
                <li
                    class="breadcrumb-item text-truncate dark:text-stone-400"
                    v-for="(item, index) in breadcrumb"
                    v-bind:key="index"
                    v-bind:class="[breadcrumb.length === index + 1 ? 'active' : '']"
                    v-on:click="selectDirectory(index)"
                >
                    <div class="flex items-center">
                        <span class="mx-2 text-stone-400">/</span>
                        <span>{{ item }}</span>
                    </div>
                </li>
            </ol>
        </nav>
    </div>
</template>

<script>
export default {
    name: 'BreadCrumb',
    props: {
        manager: { type: String, required: true },
    },
    computed: {
        /**
         * Active manager name
         * @returns {any}
         */
        activeManager() {
            return this.$store.state.fm.activeManager;
        },

        /**
         * Selected Disk for this manager
         * @returns {any}
         */
        selectedDisk() {
            return this.$store.state.fm[this.manager].selectedDisk;
        },

        /**
         * Selected directory for this manager
         * @returns {any}
         */
        selectedDirectory() {
            return this.$store.state.fm[this.manager].selectedDirectory;
        },

        /**
         * Breadcrumb
         * @returns {*}
         */
        breadcrumb() {
            return this.$store.getters[`fm/${this.manager}/breadcrumb`];
        },
    },
    methods: {
        /**
         * Load selected directory
         * @param index
         */
        selectDirectory(index) {
            const path = this.breadcrumb.slice(0, index + 1).join('/');

            // only if this path not selected
            if (path !== this.selectedDirectory) {
                // load directory
                this.$store.dispatch(`fm/${this.manager}/selectDirectory`, { path, history: true });
            }
        },

        /**
         * Select main directory
         */
        selectMainDirectory() {
            if (this.selectedDirectory) {
                this.$store.dispatch(`fm/${this.manager}/selectDirectory`, { path: null, history: true });
            }
        },
    },
};
</script>

<style lang="scss">
.fm-breadcrumb {
    @apply mb-4;

    .breadcrumb-item {
        @apply inline-flex items-center;
    }

    .breadcrumb-item:not(.active):hover {
        cursor: pointer;
        font-weight: normal;
        color: #6c757d;
    }
}
</style>
