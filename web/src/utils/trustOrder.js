// The order trust center items are shown in: sort_order, then title. The public
// page and the manager screen both sort with this so what a manager arranges is
// what visitors see. `titleOf` reads the title off a row (manager rows keep it
// under `document`).
export function compareTrustItems(a, b, titleOf = (x) => x.title) {
  return (a.sort_order - b.sort_order) || String(titleOf(a)).localeCompare(String(titleOf(b)))
}
