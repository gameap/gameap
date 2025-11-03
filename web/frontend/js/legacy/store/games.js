import axios from '../../config/axios';

const state = {
    gameCode: '',
};

const actions = {
    setGameCode({commit}, gameCode) {
        commit('setGameCode', gameCode);
    },
};

const mutations = {
    setGameCode(state, gameCode) {
        state.gameCode = gameCode;
    },
};

export default {
    namespaced: true,
    state,
    actions,
    mutations
};