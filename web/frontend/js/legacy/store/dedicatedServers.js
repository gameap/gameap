import axios from '../../config/axios';

const state = {
    dsId: 0,
    ipList: [],
    busyPorts: [],
};

const actions = {
    setDsId({commit}, dsId) {
        commit('setDsId', dsId);
    },

    resetDsId({commit}) {
        commit('setDsId', 0);
    },

    fetchIpList({state, commit}) {
        if (state.dsId <= 0) {
            return;
        }

        axios.get('/api/dedicated_servers/' + state.dsId + '/ip_list').then(function (response) {
            commit('setIpList', response.data);
        });
    },

    fetchBusyPorts({state, commit}, callback = false) {
        if (state.dsId <= 0) {
            return;
        }

        axios.get('/api/dedicated_servers/' + state.dsId + '/busy_ports').then((response) => {
            commit('setBusyPorts', response.data);

            if (typeof callback === 'function') {
                callback();
            }
        });
    }
};

const mutations = {
    setDsId (state, dsId) {
        state.dsId = dsId;
    },

    setIpList (state, ipList) {
        state.ipList = ipList;
    },

    setBusyPorts(state, busyPorts) {
        state.busyPorts = busyPorts;
    },
};

export default {
    namespaced: true,
    state,
    actions,
    mutations
};