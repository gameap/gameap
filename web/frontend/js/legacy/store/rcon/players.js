import axios from "@/config/axios"

const state = {
    supportedFeatures: null,
    players: Array,
};

const actions = {
    async fetchPlayers({state, commit, dispatch, rootState}) {
        if (rootState.servers.serverId <= 0) {
            return;
        }

        const response = await axios.get('/api/servers/' + rootState.servers.serverId + '/rcon/players');
        commit('setPlayers', response.data);
    },

    async supportedFeatures({state, commit, dispatch, rootState}) {
        if (rootState.servers.serverId <= 0 || state.supportedFeatures !== null) {
            return;
        }

        const response = await axios.get('/api/servers/' + rootState.servers.serverId + '/rcon/features');
        commit('setSupportedFeatures', response.data);
    },

    async kickPlayer({state, commit, dispatch, rootState}, {player, reason}) {
        await axios.post('/api/servers/' + rootState.servers.serverId + '/rcon/players/kick', {
            player: player,
            reason: reason
        });
        await dispatch('fetchPlayers');
    },

    async banPlayer({state, commit, dispatch, rootState}, {player, reason, time}) {
        await axios.post('/api/servers/' + rootState.servers.serverId + '/rcon/players/ban', {
            player: player,
            reason: reason,
            time: time,
        });

        await dispatch('fetchPlayers');
    },

    async sendMessage({state, commit, dispatch, rootState}, {playerId, message}) {
        await axios.post('/api/servers/' + rootState.servers.serverId + '/rcon/players/message', {
            player: playerId,
            message: message,
        });
    },
};

const mutations = {
    setPlayers(state, players) {
        state.players = players;
    },

    setSupportedFeatures(state, features) {
        state.supportedFeatures = features;
    }
}

export default {
    namespaced: true,
    state,
    actions,
    mutations
};

