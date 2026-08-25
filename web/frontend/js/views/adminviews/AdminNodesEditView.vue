<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <UpdateNodeForm
      :loading="loading"
      v-model="nodeUpdateModel"
      v-on:send="onUpdate"
      :client-certificate-options="certificateOptions"
    />
</template>

<script setup>
import { GBreadcrumbs } from "@gameap/ui"
import {computed, ref} from "vue"
import { camelCase, snakeCase } from "lodash-es"
import {trans} from "@/i18n/i18n"
import {useNodeStore} from "@/store/node"
import {useClientCertificatesStore} from "@/store/clientCertificates";
import {useInitialLoad} from "@/composables/useInitialLoad";
import {errorNotification, notification} from "@/parts/dialogs"
import {useRoute, useRouter} from "vue-router"
import {storeToRefs} from "pinia"
import UpdateNodeForm from "./forms/UpdateNodeForm.vue";

const route = useRoute()
const router = useRouter()

const nodeStore = useNodeStore()
const clientCertificatesStore = useClientCertificatesStore()

const breadcrumbs = computed(() => {
  return [
    {route:'/', text:'GameAP', icon: 'gameap'},
    {route:{name: 'admin.nodes.index'}, text:trans('dedicated_servers.dedicated_servers')},
    {text: trans('dedicated_servers.edit')},
  ]
})

const { node } = storeToRefs(nodeStore)
const { certificates } = storeToRefs(clientCertificatesStore)

// originalMetadata keeps the bag as the API returned it, so onUpdate can tell
// an untouched row from an edited one and preserve non-string values.
const originalMetadata = ref({})

const metadataValueToText = (value) => {
  return typeof value === 'object' && value !== null ? JSON.stringify(value) : String(value)
}

const loading = useInitialLoad(async () => {
  nodeStore.setNodeId(route.params.id)

  const [nodeResult, certificatesResult] = await Promise.allSettled([
    nodeStore.fetchNode(),
    clientCertificatesStore.fetchClientCertificates(),
  ])

  if (certificatesResult.status === 'rejected') {
    errorNotification(certificatesResult.reason)
  }

  if (nodeResult.status === 'rejected') {
    errorNotification(nodeResult.reason)

    return
  }

  nodeUpdateModel.value = Object.fromEntries(
      Object.entries(node.value).map(([k, v]) => [camelCase(k), v])
  );

  // The metadata bag is edited as a list of {key, value} rows; values are
  // rendered as text, so non-string entries are stringified for display.
  originalMetadata.value = {...(node.value.metadata || {})}
  nodeUpdateModel.value.metadata = Object.entries(originalMetadata.value)
      .map(([key, value]) => ({key, value: metadataValueToText(value)}))
})

const certificateOptions = computed(() => {
  return certificates.value.map((certificate) => {
    return {
      label: certificate.fingerprint,
      value: certificate.id,
    };
  })
})

const nodeUpdateModel = ref({
  name: '',
  description: '',
  location: '',
  metadata: [],
})

const onUpdate = async () => {
  if (nodeUpdateModel.value.serverCertificateFile) {
    nodeUpdateModel.value.gdaemonServerCert = await nodeUpdateModel.value.serverCertificateFile.text()
  } else {
    nodeUpdateModel.value.gdaemonServerCert = ''
  }

  const fields = Object.fromEntries(
      Object.entries(nodeUpdateModel.value).map(([k, v]) => [snakeCase(k), v])
  );

  // A row the user did not edit keeps the stored value verbatim: the editor
  // renders every value as text, so sending the text back would turn numbers,
  // booleans and nested objects into strings nobody asked to change. Edited
  // and new rows are sent as the strings they were typed as.
  const metadataObj = {}
  for (const {key, value} of nodeUpdateModel.value.metadata || []) {
    if (!key) {
      continue
    }

    const isUnchanged = Object.hasOwn(originalMetadata.value, key)
        && metadataValueToText(originalMetadata.value[key]) === value

    metadataObj[key] = isUnchanged ? originalMetadata.value[key] : value
  }
  fields.metadata = metadataObj

  nodeStore.saveNode(fields).then(() => {
    notification({
      content: trans('dedicated_servers.update_success_msg'),
      type: "success",
    }, () => {
      router.push({name: 'admin.nodes.index'})
    })
  }).catch((error) => {
    errorNotification(error)
  })
}

</script>