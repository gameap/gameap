<template>
    <ul class="list-unstyled fm-tree-branch">
        <li v-for="(directory, index) in subDirectories" v-bind:key="index">
            <p
                class="unselectable"
                v-bind:class="{ selected: isDirectorySelected(directory.path) }"
                v-on:click="selectDirectory(directory.path)"
            >
                <i
                    class="fa-regular"
                    v-if="directory.props.hasSubdirectories"
                    v-on:click.stop="showSubdirectories(directory.path, directory.props.showSubdirectories)"
                    v-bind:class="[arrowState(index) ? 'fa-square-minus' : 'fa-square-plus']"
                />
                <i class="fa-solid fa-minus" v-else />
                {{ directory.basename }}
            </p>

            <transition name="fade-tree">
                <tree-branch
                    v-show="arrowState(index)"
                    v-if="directory.props.hasSubdirectories"
                    v-bind:parent-id="directory.id"
                >
                </tree-branch>
            </transition>
        </li>
    </ul>
</template>

<script>
export default {
    name: 'TreeBranch',
    props: {
        parentId: { type: Number, required: true },
    },
    computed: {
        /**
         * Subdirectories for selected parent
         * @returns {*}
         */
        subDirectories() {
            return this.$store.getters['fm/tree/directories'].filter((item) => item.parentId === this.parentId);
        },
    },
    methods: {
        /**
         * Check, is this directory selected?
         * @param path
         * @returns {boolean}
         */
        isDirectorySelected(path) {
            return this.$store.state.fm.left.selectedDirectory === path;
        },

        /**
         * Show subdirectories - arrow
         * @returns {boolean}
         * @param index
         */
        arrowState(index) {
            return this.subDirectories[index].props.showSubdirectories;
        },

        /**
         * Show/Hide subdirectories
         * @param path
         * @param showState
         */
        showSubdirectories(path, showState) {
            if (showState) {
                // hide
                this.$store.dispatch('fm/tree/hideSubdirectories', path);
            } else {
                // show
                this.$store.dispatch('fm/tree/showSubdirectories', path);
            }
        },

        /**
         * Load selected directory and show files
         * @param path
         */
        selectDirectory(path) {
            // only if this path not selected
            if (!this.isDirectorySelected(path)) {
                this.$store.dispatch('fm/left/selectDirectory', { path, history: true });
            }
        },
    },
};
</script>

<style lang="scss">
.fm-tree-branch {
    display: table;
    width: 100%;
    padding-left: 1rem;

    li > p {
        margin-bottom: 0;
        padding: 0.4rem 0.4rem;
        white-space: nowrap;
        cursor: pointer;

        &:hover,
        &.selected {
            background-color: #f8f9fa;
        }
    }
}

.fade-tree-enter-active,
.fade-tree-leave-active {
    transition: all 0.3s ease;
}

.fade-tree-enter,
.fade-tree-leave-to {
    transform: translateX(20px);
    opacity: 0;
}
</style>
