<template>
    <div>
        <pre class="console">{{ output }}</pre>
    </div>
</template>

<script>
    export default {
        props: {
            taskId: Number,
        },
        data: function () {
            return {
                updateOutput: true,
                output: null,
            };
        },
        methods: {
            getOutput() {
                if (!this.updateOutput) {
                    return;
                }

                axios.get('/api/gdaemon_tasks/' + this.taskId + '/output')
                    .then(function(response) {
                        this.output = response.data.output;
                        if (response.data.status !== 'working') {
                            this.updateOutput = false;
                            location.reload();
                        }
                    }.bind(this))
                    .catch(function (error) {
                        console.log(error);
                        this.updateOutput = false;
                    }.bind(this));
            },
        },
        mounted() {
            this.getOutput();
            setInterval(this.getOutput, 5000);
        }
    }
</script>
