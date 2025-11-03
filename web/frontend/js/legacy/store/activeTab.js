import axios from '../../config/axios';

const state = {
    name: '',
};

const actions = {
    setName({commit}, tabName) {
        commit('setTabName', tabName);
    },
};

const mutations = {
    setTabName(state, tabName) {
        state.name = tabName;
    },
};

export default {
    namespaced: true,
    state,
    actions,
    mutations
};