<template>
    <div class="modal-content fm-modal-upload">
        <div class="modal-header grid grid-cols-2">
            <h5 class="modal-title">{{ lang.modal.upload.title }}</h5>
            <button type="button" class="btn-close" aria-label="Close" v-on:click="hideModal">
                <i class="fa-solid fa-xmark"></i>
            </button>
        </div>
        <div class="modal-body">
            <div class="fm-btn-wrapper" v-show="!progressBar">
                <button type="button" class="btn btn-secondary btn-block file-upload">
                    {{ lang.btn.uploadSelect }}
                </button>
                <input type="file" multiple name="myfile" v-on:change="selectFiles($event)" />
            </div>
            <div class="fm-upload-list" v-if="countFiles">
                <div class="grid grid-cols-2 gap-4 my-4" v-for="(item, index) in newFiles" v-bind:key="index">
                    <div class="w-75 text-truncate">
                        <i v-bind:class="mimeToIcon(item.type)" />
                        {{ item.name }}
                    </div>
                    <div class="text-right">
                        {{ bytesToHuman(item.size) }}
                    </div>
                </div>
                <hr />
                <div class="grid grid-cols-2 gap-4 my-4">
                    <div>
                        <strong>{{ lang.modal.upload.selected }}</strong>
                        {{ newFiles.length }}
                    </div>
                    <div class="text-right">
                        <strong>{{ lang.modal.upload.size }}</strong>
                        {{ allFilesSize }}
                    </div>
                </div>
                <hr />
                <div class="grid grid-cols-3 gap-4 my-3 my-4">
                    <div>
                        <strong>{{ lang.modal.upload.ifExist }}</strong>
                    </div>
                    <div class="form-check form-check-inline">
                        <input
                            class="form-check-input"
                            id="uploadRadio1"
                            type="radio"
                            name="uploadOptions"
                            v-bind:checked="!overwrite"
                            v-on:change="overwrite = 0"
                        />
                        <label class="form-check-label" for="uploadRadio1">
                            {{ lang.modal.upload.skip }}
                        </label>
                    </div>
                    <div class="form-check form-check-inline">
                        <input
                            class="form-check-input"
                            id="uploadRadio2"
                            type="radio"
                            name="uploadOptions"
                            v-bind:checked="overwrite"
                            v-on:change="overwrite = 1"
                        />
                        <label class="form-check-label" for="uploadRadio2">
                            {{ lang.modal.upload.overwrite }}
                        </label>
                    </div>
                </div>
                <hr />
            </div>
            <div v-else>
                <p>{{ lang.modal.upload.noSelected }}</p>
            </div>
            <div class="fm-upload-info my-4">
                <!-- Progress Bar -->
                <div class="progress w-full bg-stone-200 dark:bg-stone-700" v-show="countFiles">
                    <div
                        class="progress-bar progress-bar-striped bg-lime-600 text-xs font-medium text-lime-100 text-center leading-none"
                        role="progressbar"
                        v-bind:aria-valuenow="progressBar"
                        aria-valuemin="0"
                        aria-valuemax="100"
                        v-bind:style="{ width: progressBar + '%' }"
                    >
                        {{ progressBar }}%
                    </div>
                </div>
            </div>
        </div>
        <div class="modal-footer">
            <button
                type="button"
                class="btn rounded mr-2"
                v-bind:class="[countFiles ? 'btn-info' : 'btn-light']"
                v-bind:disabled="!countFiles"
                v-on:click="uploadFiles"
            >
                {{ lang.btn.submit }}
            </button>
            <button type="button" class="btn btn-light rounded" v-on:click="hideModal()">{{ lang.btn.cancel }}</button>
        </div>
    </div>
</template>

<script>
import modal from '../mixins/modal';
import translate from '../../../mixins/translate';
import helper from '../../../mixins/helper';

export default {
    name: 'UploadModal',
    mixins: [modal, translate, helper],
    data() {
        return {
            newFiles: [],
            overwrite: 0,
        };
    },
    computed: {
        /**
         * Progress bar value - %
         * @returns {number}
         */
        progressBar() {
            return this.$store.state.fm.messages.actionProgress;
        },

        /**
         * Count of files selected for upload
         * @returns {number}
         */
        countFiles() {
            return this.newFiles.length;
        },

        /**
         * Calculate the size of all files
         * @returns {*|string}
         */
        allFilesSize() {
            let size = 0;

            for (let i = 0; i < this.newFiles.length; i += 1) {
                size += this.newFiles[i].size;
            }

            return this.bytesToHuman(size);
        },
    },
    methods: {
        /**
         * Select file or files
         * @param event
         */
        selectFiles(event) {
            // files selected?
            if (event.target.files.length === 0) {
                // no file selected
                this.newFiles = [];
            } else {
                // we have file or files
                this.newFiles = event.target.files;
            }
        },

        /**
         * Upload new files
         */
        uploadFiles() {
            // if files exists
            if (this.countFiles) {
                // upload files
                this.$store
                    .dispatch('fm/upload', {
                        files: this.newFiles,
                        overwrite: this.overwrite,
                    })
                    .then((response) => {
                        if (response.data.result.status === 'success') {
                            this.hideModal();
                        }
                    });
            }
        },
    },
};
</script>

<style lang="scss">
.fm-modal-upload {
    .fm-btn-wrapper {
        position: relative;
        overflow: hidden;
        padding-bottom: 6px;
        margin-bottom: 0.6rem;
    }

    .fm-btn-wrapper input[type='file'] {
        font-size: 100px;
        position: absolute;
        left: 0;
        top: 0;
        opacity: 0;
        cursor: pointer;
    }

    .fm-upload-list .fa {
        padding-right: 0.5rem;
    }

    .fm-upload-list .form-check-inline {
        margin-right: 0;
    }

    .fm-upload-info > .progress {
        margin-bottom: 1rem;
    }

    .file-upload {
        @apply block w-full inline-block align-middle text-center select-none font-normal leading-normal no-underline text-black bg-white border border-stone-300 focus:outline-none hover:bg-stone-100 focus:ring-4 focus:ring-stone-100 dark:bg-stone-800 dark:text-white dark:border-stone-600 dark:hover:bg-stone-700 dark:hover:border-stone-600 dark:focus:ring-stone-700;
    }
}
</style>
