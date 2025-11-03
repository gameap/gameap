import mutations from './mutations.js';

export default {
    namespaced: true,
    state() {
        return {
            // modal window
            showModal: false,

            // modal name
            modalName: null,

            // main modal block height
            modalBlockHeight: 0,
        };
    },
    mutations,
};
