# `isms.sh/demo`

SDK for seeding demonstration organisations into a running ISMS platform.

This package ships **no organisation-specific content** — only the plumbing
(`Seeder` + `Content` interface + `Run` orchestrator). Per-customer demo
content lives in separate repositories that import this package.

The canonical consumer is [`unidoc/isms-demo`](https://github.com/unidoc/isms-demo)
— see that repo for a complete content package and a working CLI wired around
`demo.Run`.
