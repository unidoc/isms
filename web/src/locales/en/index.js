// The `en` message bundle: one file per area, merged into a single nested
// keyspace. Area = the top-level key, and it is also the filename — so
// `risks.table.header.likelihood` will live in `risks.json` and nowhere else.
//
// The split is not cosmetic. Extraction is one PR per view, and a single
// en.json would make ~21 concurrent PRs collide on one file. Each area file
// therefore arrives with the PR that extracts that area: one new file plus one
// import line here. Only `common.json` exists up front, because the shared
// vocabulary is what every area draws on.
//
// Import attributes (`with { type: 'json' }`) are required by Node for JSON
// modules, and understood by Vite/Rollup — the same file therefore loads both
// in the bundler and under `node --test`.
import common from './common.json' with { type: 'json' }
import notifications from './notifications.json' with { type: 'json' }

export default {
  common,
  notifications,
}
