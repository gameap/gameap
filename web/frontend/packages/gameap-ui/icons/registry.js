import { markRaw, reactive } from 'vue'
import { defaultIconMap } from './iconMap.js'

// Icon values are components or plain SVG data — they never change, so they are
// kept out of the reactivity system. Only adding entries has to be reactive.
function raw(icons) {
  return Object.fromEntries(
    Object.entries(icons).map(([name, icon]) => [
      name,
      typeof icon === 'object' || typeof icon === 'function' ? markRaw(icon) : icon,
    ]),
  )
}

const iconRegistry = reactive(raw(defaultIconMap))

export function registerIcons(icons) {
  Object.assign(iconRegistry, raw(icons))
}

export function getIcon(name) {
  return iconRegistry[name]
}

export function hasIcon(name) {
  return name in iconRegistry
}

export { iconRegistry }
