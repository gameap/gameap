import { defineSvgIcon } from './svgData.js'

/**
 * Loads every SVG in ./assets as icon data. The files are the single source of
 * truth — drop one in and it becomes available under its file name.
 *
 * A file must have a viewBox and must not set `fill`, so that the icon inherits
 * `currentColor`. At most one wrapping <g transform="..."> is supported; logos
 * extracted from the old gicon font use it to flip the y axis, because SVG fonts
 * store glyphs with the y axis pointing up.
 */
const sources = import.meta.glob('./assets/*.svg', {
  query: '?raw',
  import: 'default',
  eager: true,
})

function parse(name, source) {
  const viewBox = /<svg\b[^>]*\bviewBox="([^"]+)"/.exec(source)?.[1]

  if (!viewBox) {
    throw new Error(`icon "${name}": no viewBox`)
  }

  const paths = [...source.matchAll(/<path\b[^>]*\bd="([^"]+)"/g)].map(([, path]) => path)

  if (paths.length === 0) {
    throw new Error(`icon "${name}": no paths`)
  }

  const transforms = [...source.matchAll(/<g\b[^>]*\btransform="([^"]+)"/g)].map(([, value]) => value)

  if (transforms.length > 1) {
    throw new Error(`icon "${name}": more than one transformed group`)
  }

  return defineSvgIcon({ viewBox, transform: transforms[0], paths })
}

export const svgAssets = Object.fromEntries(
  Object.entries(sources).map(([path, source]) => {
    const name = path.slice(path.lastIndexOf('/') + 1, -'.svg'.length)

    return [name, parse(name, source)]
  }),
)

export function svgAsset(name) {
  if (!(name in svgAssets)) {
    throw new Error(`unknown SVG asset "${name}" — expected icons/assets/${name}.svg`)
  }

  return svgAssets[name]
}
