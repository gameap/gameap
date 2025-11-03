// store modules
import tree from './store/tree/store.js';
import modal from './store/modal/store.js';
import settings from './store/settings/store.js';
import manager from './store/manager/store.js';
import messages from './store/messages/store.js';
// main store
import state from './store/state.js';
import mutations from './store/mutations.js';
import getters from './store/getters.js';
import actions from './store/actions.js';

export default {
    namespaced: true,
    modules: {
        settings,
        left: manager,
        right: manager,
        tree,
        modal,
        messages,
    },
    state,
    mutations,
    actions,
    getters,
};
