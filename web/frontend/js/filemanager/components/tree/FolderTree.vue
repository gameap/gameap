<template>
    <div class="fm-tree">
        <div class="fm-tree-disk sticky-top"><i class="fa-solid fa-hard-drive"></i> {{ selectedDisk }}</div>
        <branch v-bind:parent-id="0" />
    </div>
</template>

<script>
import Branch from './TreeBranch.vue';

export default {
    name: 'FolderTree',
    components: {
        branch: Branch,
    },
    computed: {
        /**
         * Selected Disk
         * @returns {*}
         */
        selectedDisk() {
            return this.$store.getters['fm/selectedDisk'];
        },
    },
};
</script>

<style lang="scss">
.fm-tree {
    overflow: auto;
    border-right: 1px solid #6c757d;

    @media screen and (min-height: 800px) {
        height: 550px;
    }
    @media screen and (max-height: 800px) {
        height: 420px;
    }
    @media screen and (max-height: 600px) {
        height: 350px;
    }

    & > .fm-tree-branch {
        padding-left: 0;
    }

    .fm-tree-disk {
        padding: 0.2rem 0.3rem;
        background-color: #cff4fc;
    }

    .fm-tree-disk > i {
        padding-right: 0.4rem;
    }
}
</style>
