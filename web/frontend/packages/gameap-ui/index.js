export { default as GBreadcrumbs } from './components/GBreadcrumbs.vue'
export { default as GDeletableList } from './components/GDeletableList.vue'
export { default as GStatusBadge } from './components/GStatusBadge.vue'
export { default as Loading } from './components/Loading.vue'
export { default as Progressbar } from './components/Progressbar.vue'
export { default as GMenu } from './components/GMenu.vue'
export { default as GMenuButton } from './components/GMenuButton.vue'
export { default as GMenuItems } from './components/GMenuItems.vue'
export { default as GMenuItem } from './components/GMenuItem.vue'
export { default as GIcon } from './components/GIcon.vue'
export { default as GDataTable } from './components/GDataTable.vue'
export { default as GModal } from './components/GModal.vue'
export { default as GInput } from './components/GInput.vue'
export { default as GCard } from './components/GCard.vue'
export { default as GTable } from './components/GTable.vue'
export { default as GEmpty } from './components/GEmpty.vue'
export { default as GSwitch } from './components/GSwitch.vue'
export { default as GDivider } from './components/GDivider.vue'
export { default as GGameIcon } from './components/GGameIcon.vue'

export { registerIcons, iconRegistry, getIcon, hasIcon } from './icons/registry.js'
export { defaultIconMap } from './icons/iconMap.js'
export { defineSvgIcon, isSvgIcon } from './icons/svgData.js'

import GBreadcrumbs from './components/GBreadcrumbs.vue'
import GDeletableList from './components/GDeletableList.vue'
import GStatusBadge from './components/GStatusBadge.vue'
import Loading from './components/Loading.vue'
import Progressbar from './components/Progressbar.vue'
import GMenu from './components/GMenu.vue'
import GMenuButton from './components/GMenuButton.vue'
import GMenuItems from './components/GMenuItems.vue'
import GMenuItem from './components/GMenuItem.vue'
import GIcon from './components/GIcon.vue'
import GDataTable from './components/GDataTable.vue'
import GModal from './components/GModal.vue'
import GInput from './components/GInput.vue'
import GCard from './components/GCard.vue'
import GTable from './components/GTable.vue'
import GEmpty from './components/GEmpty.vue'
import GSwitch from './components/GSwitch.vue'
import GDivider from './components/GDivider.vue'
import GGameIcon from './components/GGameIcon.vue'

export function install(app) {
  app.component('GBreadcrumbs', GBreadcrumbs)
  app.component('GDeletableList', GDeletableList)
  app.component('GStatusBadge', GStatusBadge)
  app.component('Loading', Loading)
  app.component('Progressbar', Progressbar)
  app.component('GMenu', GMenu)
  app.component('GMenuButton', GMenuButton)
  app.component('GMenuItems', GMenuItems)
  app.component('GMenuItem', GMenuItem)
  app.component('GIcon', GIcon)
  app.component('GDataTable', GDataTable)
  app.component('GModal', GModal)
  app.component('GInput', GInput)
  app.component('GCard', GCard)
  app.component('GTable', GTable)
  app.component('GEmpty', GEmpty)
  app.component('GSwitch', GSwitch)
  app.component('GDivider', GDivider)
  app.component('GGameIcon', GGameIcon)
}

export default { install }
