<template>
    <transition name="fm-modal">
        <div class="fm-modal modal z-50 hs-overlay-backdrop transition duration fixed inset-0 bg-stone-900 bg-opacity-50 dark:bg-opacity-80 dark:bg-neutral-900" aria-hidden="true" ref="fmModal" v-on:click="hideModal">
            <div id="static-modal" tabindex="-1" class="absolute inset-0 h-screen flex justify-center items-center overflow-y-auto overflow-x-hidden fixed top-0 right-0 left-0 justify-center items-center w-full md:inset-0 h-[calc(100%-1rem)] max-h-full">
                <div class="relative p-4 w-full max-w-2xl max-h-full" v-bind:class="modalSize" >
                    <div class="relative bg-white rounded-lg shadow dark:bg-stone-700">
                        <div class="p-4 md:p-5 space-y-4">
                            <div class="modal-dialog modal-dialog-centered" role="document" v-on:click.stop>
                                <component v-bind:is="modalName" />
                            </div>
                        </div>

                    </div>
                </div>
            </div>
        </div>
    </transition>
</template>

<script>
import NewFileModal from './views/NewFileModal.vue';
import NewFolderModal from './views/NewFolderModal.vue';
import UploadModal from './views/UploadModal.vue';
import DeleteModal from './views/DeleteModal.vue';
import ClipboardModal from './views/ClipboardModal.vue';
import StatusModal from './views/StatusModal.vue';
import RenameModal from './views/RenameModal.vue';
import PropertiesModal from './views/PropertiesModal.vue';
import PreviewModal from './views/PreviewModal.vue';
import TextEditModal from './views/TextEditModal.vue';
import AudioPlayerModal from './views/AudioPlayerModal.vue';
import VideoPlayerModal from './views/VideoPlayerModal.vue';
import ZipModal from './views/ZipModal.vue';
import UnzipModal from './views/UnzipModal.vue';
import AboutModal from './views/AboutModal.vue';

export default {
    name: 'ModalBlock',
    components: {
        NewFileModal,
        NewFolderModal,
        UploadModal,
        DeleteModal,
        ClipboardModal,
        StatusModal,
        RenameModal,
        PropertiesModal,
        PreviewModal,
        TextEditModal,
        AudioPlayerModal,
        VideoPlayerModal,
        ZipModal,
        UnzipModal,
        AboutModal,
    },
    mounted() {
        // set height
        this.$store.commit('fm/modal/setModalBlockHeight', this.$refs.fmModal.offsetHeight);
    },
    computed: {
        /**
         * Selected modal name
         * @returns {null|*}
         */
        modalName() {
            return this.$store.state.fm.modal.modalName;
        },

        /**
         * Modal size style
         * @returns {{'modal-lg': boolean, 'modal-sm': boolean}}
         */
        modalSize() {
            return {
                'modal-xl': this.modalName === 'PreviewModal' || this.modalName === 'TextEditModal',
                'modal-lg': this.modalName === 'VideoPlayerModal',
                'modal-sm': false,
            };
        },
    },
    methods: {
        /**
         * Hide modal window
         */
        hideModal() {
            this.$store.commit('fm/modal/clearModal');
        },
    },
};
</script>

<style lang="scss">
.fm-modal {
    .modal-xl {
        max-width: 1000px;
    }
}

</style>
