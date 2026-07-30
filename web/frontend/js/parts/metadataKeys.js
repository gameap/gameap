// Metadata keys grouped by what reads them: the panel itself and the daemon
// process managers (docker, podman). Group titles and per-key descriptions live
// in i18n under `metadata_keys.*`, examples stay here as-is.
export const metadataKeyGroups = [
    {
        name: 'panel',
        keys: [
            {key: 'public_ip', example: '203.0.113.10'},
        ],
    },
    {
        name: 'container',
        keys: [
            {key: 'docker_image', example: 'gameap/srcds:latest'},
            {key: 'docker_container_name', example: 'csgo-competitive'},
            {key: 'docker_workdir', example: '/home/container'},
            {key: 'docker_network_mode', example: 'host'},
            {key: 'docker_dns', example: '8.8.8.8,1.1.1.1'},
            {key: 'docker_volumes', example: '/srv/maps:/maps:ro'},
            {key: 'docker_capabilities', example: 'NET_RAW,SYS_NICE'},
            {key: 'docker_privileged', example: 'true'},
            {key: 'docker_installation_image', example: 'ghcr.io/parkervcp/installers:alpine'},
            {key: 'docker_installation_script', example: '#!/bin/ash\nset -e\napk add --no-cache curl'},
            {key: 'docker_installation_entrypoint', example: 'ash'},
            {key: 'docker_installation_user', example: 'root'},
        ],
    },
]
