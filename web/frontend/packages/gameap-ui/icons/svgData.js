/**
 * Describes an icon as raw SVG data rendered by GIcon.
 *
 * `transform` is only needed for icons extracted from an SVG font, where the
 * y axis points up and the glyph has to be mirrored.
 */
export function defineSvgIcon({ viewBox = '0 0 24 24', paths, transform }) {
  const [, , width, height] = viewBox.split(/[\s,]+/).map(Number)

  return { __svgIcon: true, viewBox, paths, transform, aspect: width / height }
}

export function isSvgIcon(icon) {
  return typeof icon === 'object' && icon !== null && icon.__svgIcon === true
}
