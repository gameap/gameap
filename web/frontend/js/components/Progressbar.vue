<template>
    <div class="progress">
      <div class="progress-bar progress-bar-info" role="progressbar" v-bind:aria-valuenow="progress"
      aria-valuemin="0" aria-valuemax="100" v-bind:style="{ width: progress + '%'}">
        {{progress}}%
      </div>
    </div>
</template>

<script>
    export default {
        props: ['progress'],
        methods: {
            setProgress: function(progress) {
                this.progress = progress;
            }
        }
    }
</script>
