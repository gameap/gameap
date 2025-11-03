<template>
    <div>
        <div id="serverForm" class="mb-3">
          <label for="server-id" class="control-label">{{ trans('labels.game_server') }}</label>
          <select id="server-id" v-bind:name="serverIdFieldName" class="form-select" v-model="selectedServerId">
              <option v-for="server in serversList" v-bind:value="server.id">{{ server.name }}&nbsp;&nbsp;&nbsp;&nbsp;{{ server.server_ip }}:{{ server.server_port }}</option>
          </select>
        </div>
    </div>
</template>

<script>
    import { mapState } from 'vuex';

    export default {
        name: "ServerSelector",
        props: {
            serverIdFieldName: {
              type: String,
              default: 'game_server_id',
            }
        },
        mounted() {
            this.selectedServerId = this.selectedServerId || -1;
        },
        computed: {
            ...mapState({
                dsId: state => state.dedicatedServers.dsId,
                serversList: state => state.servers.serversList,
            }),
            selectedServerId: {
                get() { return this.$store.state.servers.serverId},
                set(serverId) { this.$store.dispatch('servers/setServerId', serverId)}
            }
        },
        watch: {
            dsId() {
                  this.$store.dispatch('servers/fetchServers');
            }
        }

    }
</script>
