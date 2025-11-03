<template>
    <div class="fm-navbar mb-3">
        <div class="grid grid-cols-2 gap-4">
            <div class="mb-2">
                <div class="btn-group mr-4 mt-2 mt-2" role="group">
<!--                    <button-->
<!--                        type="button"-->
<!--                        class="btn btn-secondary rounded-s border-r"-->
<!--                        v-bind:disabled="backDisabled"-->
<!--                        v-bind:title="lang.btn.back"-->
<!--                        v-on:click="historyBack()"-->
<!--                    >-->
<!--                        <i class="fa-solid fa-chevron-left"></i>-->
<!--                    </button>-->
<!--                    <button-->
<!--                        type="button"-->
<!--                        class="btn btn-secondary border-r"-->
<!--                        v-bind:disabled="forwardDisabled"-->
<!--                        v-bind:title="lang.btn.forward"-->
<!--                        v-on:click="historyForward()"-->
<!--                    >-->
<!--                        <i class="fa-solid fa-chevron-right"></i>-->
<!--                    </button>-->
                    <button
                        type="button"
                        class="btn btn-secondary rounded"
                        v-on:click="refreshAll()"
                        v-bind:title="lang.btn.refresh"
                    >
                        <i class="fa-solid fa-rotate"></i>
                    </button>
                </div>
                <div class="btn-group mr-4 mt-2" role="group">
                    <button
                        type="button"
                        class="btn btn-secondary rounded-s border-r"
                        v-on:click="showModal('NewFileModal')"
                        v-bind:title="lang.btn.file"
                    >
                        <i class="fa-regular fa-file"></i>
                    </button>
                    <button
                        type="button"
                        class="btn btn-secondary border-r"
                        v-on:click="showModal('NewFolderModal')"
                        v-bind:title="lang.btn.folder"
                    >
                        <i class="fa-regular fa-folder"></i>
                    </button>
                    <button
                        type="button"
                        class="btn btn-secondary border-r"
                        disabled
                        v-if="uploading"
                        v-bind:title="lang.btn.upload"
                    >
                        <i class="fa-solid fa-upload"></i>
                    </button>
                    <button
                        type="button"
                        class="btn btn-secondary border-r"
                        v-else
                        v-on:click="showModal('UploadModal')"
                        v-bind:title="lang.btn.upload"
                    >
                        <i class="fa-solid fa-upload"></i>
                    </button>
                    <button
                        type="button"
                        class="btn btn-secondary rounded-e"
                        v-bind:disabled="!isAnyItemSelected"
                        v-on:click="showModal('DeleteModal')"
                        v-bind:title="lang.btn.delete"
                    >
                        <i class="fa-regular fa-trash-can"></i>
                    </button>
                </div>
                <div class="btn-group mr-4 mt-2" role="group">
                    <button
                        type="button"
                        class="btn btn-secondary rounded-s border-r"
                        v-bind:disabled="!isAnyItemSelected"
                        v-bind:title="lang.btn.copy"
                        v-on:click="toClipboard('copy')"
                    >
                        <i class="fa-regular fa-copy"></i>
                    </button>
                    <button
                        type="button"
                        class="btn btn-secondary border-r"
                        v-bind:disabled="!isAnyItemSelected"
                        v-bind:title="lang.btn.cut"
                        v-on:click="toClipboard('cut')"
                    >
                        <i class="fa-solid fa-scissors"></i>
                    </button>
                    <button
                        type="button"
                        class="btn btn-secondary rounded-e"
                        v-bind:disabled="!clipboardType"
                        v-bind:title="lang.btn.paste"
                        v-on:click="paste"
                    >
                        <i class="fa-regular fa-paste"></i>
                    </button>
                </div>
            </div>
            <div class="text-right">
                <div class="btn-group mr-4 mt-2" role="group">
                    <button
                        type="button"
                        class="btn btn-secondary rounded-s border-r"
                        v-bind:class="[viewType === 'table' ? 'active' : '']"
                        v-on:click="selectView('table')"
                        v-bind:title="lang.btn.table"
                    >
                        <i class="fa-solid fa-list"></i>
                    </button>
                    <button
                        type="button"
                        class="btn btn-secondary rounded-e"
                        v-bind:class="[viewType === 'grid' ? 'active' : '']"
                        v-on:click="selectView('grid')"
                        v-bind:title="lang.btn.grid"
                    >
                        <i class="fa-solid fa-grip"></i>
                    </button>
                </div>
            </div>
        </div>
    </div>
</template>

<script>
import translate from '../../mixins/translate.js';
import EventBus from '../../emitter.js';

export default {
    name: 'NavbarBlock',
    mixins: [translate],
    computed: {
        /**
         * Active manager name
         * @returns {any}
         */
        activeManager() {
            return this.$store.state.fm.activeManager;
        },

        /**
         * Back button state
         * @returns {boolean}
         */
        backDisabled() {
            return !this.$store.state.fm[this.activeManager].historyPointer;
        },

        /**
         * Forward button state
         * @returns {boolean}
         */
        forwardDisabled() {
            return (
                this.$store.state.fm[this.activeManager].historyPointer ===
                this.$store.state.fm[this.activeManager].history.length - 1
            );
        },

        /**
         * Is any files or directories selected?
         * @returns {boolean}
         */
        isAnyItemSelected() {
            return (
                this.$store.state.fm[this.activeManager].selected.files.length > 0 ||
                this.$store.state.fm[this.activeManager].selected.directories.length > 0
            );
        },

        /**
         * Manager view type - grid or table
         * @returns {any}
         */
        viewType() {
            return this.$store.state.fm[this.activeManager].viewType;
        },

        /**
         * Upload files
         * @returns {boolean}
         */
        uploading() {
            return this.$store.state.fm.messages.actionProgress > 0;
        },

        /**
         * Clipboard - action type
         * @returns {null}
         */
        clipboardType() {
            return this.$store.state.fm.clipboard.type;
        },

        /**
         * Full screen status
         * @returns {any}
         */
        fullScreen() {
            return this.$store.state.fm.fullScreen;
        },

        /**
         * Show or Hide hidden files
         * @returns {boolean}
         */
        hiddenFiles() {
            return this.$store.state.fm.settings.hiddenFiles;
        },
    },
    methods: {
        /**
         * Refresh file manager
         */
        refreshAll() {
            this.$store.dispatch('fm/refreshAll');
        },

        /**
         * History back
         */
        historyBack() {
            this.$store.dispatch(`fm/${this.activeManager}/historyBack`);
        },

        /**
         * History forward
         */
        historyForward() {
            this.$store.dispatch(`fm/${this.activeManager}/historyForward`);
        },

        /**
         * Copy
         * @param type
         */
        toClipboard(type) {
            this.$store.dispatch('fm/toClipboard', type);

            // show notification
            if (type === 'cut') {
                EventBus.emit('addNotification', {
                    status: 'success',
                    message: this.lang.notifications.cutToClipboard,
                });
            } else if (type === 'copy') {
                EventBus.emit('addNotification', {
                    status: 'success',
                    message: this.lang.notifications.copyToClipboard,
                });
            }
        },

        /**
         * Paste
         */
        paste() {
            this.$store.dispatch('fm/paste');
        },

        /**
         * Set Hide or Show hidden files
         */
        toggleHidden() {
            this.$store.commit('fm/settings/toggleHiddenFiles');
        },

        /**
         * Show modal window
         * @param modalName
         */
        showModal(modalName) {
            // show selected modal
            this.$store.commit('fm/modal/setModalState', {
                modalName,
                show: true,
            });
        },

        /**
         * Select view type
         * @param type
         */
        selectView(type) {
            if (this.viewType !== type) this.$store.commit(`fm/${this.activeManager}/setView`, type);
        },

        /**
         * Full screen toggle
         */
        screenToggle() {
            const fm = document.getElementsByClassName('fm')[0];

            if (!this.fullScreen) {
                if (fm.requestFullscreen) {
                    fm.requestFullscreen();
                } else if (fm.mozRequestFullScreen) {
                    fm.mozRequestFullScreen();
                } else if (fm.webkitRequestFullscreen) {
                    fm.webkitRequestFullscreen();
                } else if (fm.msRequestFullscreen) {
                    fm.msRequestFullscreen();
                }
            } else if (document.exitFullscreen) {
                document.exitFullscreen();
            } else if (document.webkitExitFullscreen) {
                document.webkitExitFullscreen();
            } else if (document.mozCancelFullScreen) {
                document.mozCancelFullScreen();
            } else if (document.msExitFullscreen) {
                document.msExitFullscreen();
            }

            this.$store.commit('fm/screenToggle');
        },
    },
};
</script>

<style lang="scss">
.fm-navbar {
    flex: 0 0 auto;

    .col-auto > .btn-group:not(:last-child) {
        margin-right: 0.4rem;
    }
}
</style>
