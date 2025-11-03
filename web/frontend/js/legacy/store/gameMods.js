import axios from '../../config/axios';

const state = {
    gameMod: 0,
    gameModsList: [],
};

const actions = {
    fetchGameModsList({commit}, gameId) {
        if (typeof gameId == 'undefined' || !gameId) {
            return;
        }

        axios.get('/api/game_mods/get_list_for_game/' + gameId).then(function (response) {
            commit('setModsList', response.data);
        }.bind(this));
    },

    setGameMod({commit}, gameMod) {
        commit('setGameMod', gameMod);
    }
};

const mutations = {
    setModsList (state, modsList) {
        state.gameModsList = modsList;
    },

    setGameMod(state, gameMod) {
        state.gameMod = gameMod;
    }
};

export default {
    namespaced: true,
    state,
    actions,
    mutations
};