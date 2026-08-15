// Plugins used to pass Font Awesome class strings where an icon registry name is
// now expected. Delete this module together with the fallback branch in GIcon.vue
// once the deprecation window closes.

// A style alias (fa, fas, fa-solid, ...) or any fa- prefixed icon or modifier
// class (fa-sliders, fa-fw, fa-2x). One such token anywhere in the string is
// enough, so class strings mixing Font Awesome with other classes keep working.
const FONT_AWESOME_CLASS = /(?:^|\s)(?:fa[srlbdt]?|fa-[a-z0-9-]+)(?:\s|$)/

const warned = new Set()

export function isLegacyFontAwesomeClass(name) {
  return FONT_AWESOME_CLASS.test(name)
}

export function warnLegacyFontAwesomeIcon(name) {
  if (warned.has(name)) {
    return
  }

  warned.add(name)

  console.warn(
    `GIcon: Font Awesome icon classes are deprecated and will be removed in a future release. ` +
    `Replace "${name}" with an icon name from the @gameap/ui registry, ` +
    `or register your own via registerIcons().`,
  )
}
