<template>
    <div class="modal-content fm-modal-delete">
        <div class="modal-header grid grid-cols-2">
            <h5 class="modal-title">{{ lang.modal.delete.title }}</h5>
            <button type="button" class="btn-close" aria-label="Close" v-on:click="hideModal">
                <i class="fa-solid fa-xmark"></i>
            </button>
        </div>
        <div class="modal-body">
            <div v-if="selectedItems.length">
                <selected-file-list />
            </div>
            <div v-else>
                <span class="text-danger">{{ lang.modal.delete.noSelected }}</span>
            </div>
        </div>
        <div class="modal-footer">
            <button type="button" class="btn btn-danger mr-2 rounded" v-on:click="deleteItems">{{ lang.modal.delete.title }}</button>
            <button type="button" class="btn btn-light rounded" v-on:click="hideModal">{{ lang.btn.cancel }}</button>
        </div>
    </div>
</template>

<script>
import SelectedFileList from '../additions/SelectedFileList.vue';
import modal from '../mixins/modal.js';
import translate from '../../../mixins/translate.js';

export default {
    name: 'DeleteModal',
    mixins: [modal, translate],
    components: { SelectedFileList },
    computed: {
        /**
         * Files and folders for deleting
         * @returns {*}
         */
        selectedItems() {
            return this.$store.getters['fm/selectedItems'];
        },
    },
    methods: {
        /**
         * Delete selected directories and files
         */
        deleteItems() {
            // create items list for delete
            const items = this.selectedItems.map((item) => ({
                path: item.path,
                type: item.type,
            }));

            this.$store.dispatch('fm/delete', items).then(() => {
                this.hideModal();
            });
        },
    },
};
</script>
