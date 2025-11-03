<template>
    <div class="fm-table">
        <table class="table table-sm w-full">
            <thead>
                <tr>
                    <th class="w-65" v-on:click="sortBy('name')">
                        {{ lang.manager.table.name }}
                        <template v-if="sortSettings.field === 'name'">
                            <i class="fa-solid fa-arrow-down-wide-short" v-show="sortSettings.direction === 'down'" />
                            <i class="fa-solid fa-arrow-up-wide-short" v-show="sortSettings.direction === 'up'" />
                        </template>
                    </th>
                    <th class="w-10" v-on:click="sortBy('size')">
                        {{ lang.manager.table.size }}
                        <template v-if="sortSettings.field === 'size'">
                            <i class="fa-solid fa-arrow-down-wide-short" v-show="sortSettings.direction === 'down'" />
                            <i class="fa-solid fa-arrow-up-wide-short" v-show="sortSettings.direction === 'up'" />
                        </template>
                    </th>
                    <th class="w-10" v-on:click="sortBy('type')">
                        {{ lang.manager.table.type }}
                        <template v-if="sortSettings.field === 'type'">
                            <i class="fa-solid fa-arrow-down-wide-short" v-show="sortSettings.direction === 'down'" />
                            <i class="fa-solid fa-arrow-up-wide-short" v-show="sortSettings.direction === 'up'" />
                        </template>
                    </th>
                    <th class="w-auto" v-on:click="sortBy('date')">
                        {{ lang.manager.table.date }}
                        <template v-if="sortSettings.field === 'date'">
                            <i class="fa-solid fa-arrow-down-wide-short" v-show="sortSettings.direction === 'down'" />
                            <i class="fa-solid fa-arrow-up-wide-short" v-show="sortSettings.direction === 'up'" />
                        </template>
                    </th>
                </tr>
            </thead>
            <tbody>
                <tr v-if="!isRootPath">
                    <td colspan="4" class="fm-content-item" v-on:click="levelUp">
                        <i class="fa-solid fa-arrow-turn-up"></i>
                    </td>
                </tr>
                <tr
                    v-for="(directory, index) in directories"
                    v-bind:key="`d-${index}`"
                    v-bind:class="{ 'table-info': checkSelect('directories', directory.path) }"
                    v-on:click="selectItem('directories', directory.path, $event)"
                    v-on:contextmenu.prevent="contextMenu(directory, $event)"
                >
                    <td
                        class="fm-content-item unselectable"
                        v-bind:class="acl && directory.acl === 0 ? 'text-hidden' : ''"
                        v-on:dblclick="selectDirectory(directory.path)"
                    >
                        <i class="fa-regular fa-folder"></i> {{ directory.basename }}
                    </td>
                    <td />
                    <td>{{ lang.manager.table.folder }}</td>
                    <td>
                        {{ timestampToDate(directory.timestamp) }}
                    </td>
                </tr>
                <tr
                    v-for="(file, index) in files"
                    v-bind:key="`f-${index}`"
                    v-bind:class="{ 'table-info': checkSelect('files', file.path) }"
                    v-on:click="selectItem('files', file.path, $event)"
                    v-on:dblclick="selectAction(file.path, file.extension)"
                    v-on:contextmenu.prevent="contextMenu(file, $event)"
                >
                    <td class="fm-content-item unselectable" v-bind:class="acl && file.acl === 0 ? 'text-hidden' : ''">
                        <i v-bind:class="extensionToIcon(file.extension)" />
                        {{ file.basename }}
                    </td>
                    <td>{{ bytesToHuman(file.size) }}</td>
                    <td>
                        {{ file.extension }}
                    </td>
                    <td>
                        {{ timestampToDate(file.timestamp) }}
                    </td>
                </tr>
            </tbody>
        </table>
    </div>
</template>

<script>
import translate from '../../mixins/translate';
import helper from '../../mixins/helper';
import managerHelper from './mixins/manager';

export default {
    name: 'table-view',
    mixins: [translate, helper, managerHelper],
    props: {
        manager: { type: String, required: true },
    },
    computed: {
        /**
         * Sort settings
         * @returns {*}
         */
        sortSettings() {
            return this.$store.state.fm[this.manager].sort;
        },
    },
    methods: {
        /**
         * Sort by field
         * @param field
         */
        sortBy(field) {
            this.$store.dispatch(`fm/${this.manager}/sortBy`, { field, direction: null });
        },
    },
};
</script>

<style lang="scss">
.fm-table {
    thead th {
        @apply text-left bg-white dark:bg-stone-800;

        position: sticky;
        top: 0;
        z-index: 10;
        cursor: pointer;
        border-top: none;

        &:hover {
          @apply bg-stone-100 dark:bg-[#262322];
        }

        & > i {
            padding-left: 0.5rem;
        }
    }

    td {
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    tr {
        @apply border-b dark:border-stone-700;
    }

    tr:hover {
      @apply bg-stone-100 dark:bg-stone-900;
    }

    .w-10 {
        width: 10%;
    }

    .w-65 {
        width: 65%;
    }

    .fm-content-item {
        @apply px-2 py-3;
        cursor: pointer;
    }

    .text-hidden {
        color: #cdcdcd;
    }
}
</style>
