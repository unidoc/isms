// Which register items a heat map plots. Shared by HeatMap.vue (the Risks,
// Dashboard and Legal maps) and the Dashboard's high/critical risk count, so
// the map and the count cannot disagree about what is live exposure.

// isLiveExposure reports whether an item still represents current exposure.
// A closed risk or legal requirement was treated and the treatment confirmed,
// so it is history, not something the map should show as current (#360).
// Drafts stay in: they are identified exposure that has not been dealt with.
export function isLiveExposure(item) {
  return item?.status !== 'closed'
}

// plottableItems keeps the items a 5×5 map can place: live exposure with a
// current likelihood and impact both in 1–5.
export function plottableItems(items) {
  return (Array.isArray(items) ? items : []).filter(i =>
    isLiveExposure(i) &&
    i.current_likelihood >= 1 && i.current_likelihood <= 5 &&
    i.current_impact >= 1 && i.current_impact <= 5
  )
}
