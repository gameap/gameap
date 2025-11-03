import { createStore } from 'vuex';
import dedicatedServers from './dedicatedServers';
import servers from './servers';
import games from './games';
import gameMods from './gameMods';
import activeTab from './activeTab';
import axios from '../../config/axios';

// Rcon
import rconPlayers from './rcon/players';

const store = createStore({
    modules: {
        dedicatedServers,
        servers,
        games,
        gameMods,

        activeTab,

        // Rcon
        rconPlayers,
    },
});

export default store;
