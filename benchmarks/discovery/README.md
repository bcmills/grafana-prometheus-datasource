# Discovery benchmark suite

Created: 2026-09-18T17:40:00+02:00

Compare Prometheus Search API and Labels API for metric names, label names,
and label values. The suite generates a truth manifest from a fixed seed,
serves deterministic 10k and 100k active-series datasets, and records paired
API and UI trials.

This pull request adds the harness. Do not treat a green unit test run as a
default-API decision. Run the full 10k and 100k matrix after review.

## Layout

- `config/scenarios.yaml` is the trial matrix. `smoke` is a 200-series subset.
  `full` is the 10k and 100k matrix from the remediation plan.
- `cmd/dataset` writes `manifest.json`, `active.prom`, and `historical.om`.
- `cmd/runner` issues paired Search and Labels requests and writes JSONL.
- `cmd/report` prints median, p95, p99, relevance scores, and acceptance gates.
- `tests/benchmarks/discovery.spec.ts` measures Metrics Explorer first render,
  completion, heap, and stale-result cancellation. It stays skipped unless
  `BENCH_DISCOVERY=1`.

## Dataset shapes

Each scale has three cardinality shapes so series count is not confused with
metric-name count:

- `metric-heavy`: one metric name per series.
- `value-heavy`: one metric and one `instance_id` value per series.
- `mixed`: many metrics plus high-cardinality `instance_id`, `pod`,
  `container`, and `route` values.

Storage profiles:

- `active`: samples in Prometheus head.
- `historical`: OpenMetrics blocks ending at least six hours ago.
- `mixed`: disjoint active and historical names and values.

The generator plants exact, prefix, infix, multi-word, typo, rare, UTF-8, and
no-match cases, plus `extra_label_name1` through `extra_label_name6` and
`datasource_uid` values.

## Commands

Generate the smoke fixture:

```bash
npm run bench:discovery:data -- generate --scale tiny --shape mixed --storage active
```

Build historical blocks when the profile includes them:

```bash
./benchmarks/discovery/scripts/build-historical.sh benchmarks/discovery/out/tiny-mixed-historical
```

Start the isolated stack from the repository root. Stop the default devenv
stack first. Both bind ports 3000 and 9090.

```bash
docker compose --project-directory . -f benchmarks/discovery/docker-compose.benchmark.yaml up --build
```

Confirm the live series count matches the manifest:

```bash
npm run bench:discovery:data -- validate --manifest benchmarks/discovery/out/tiny-mixed-active/manifest.json --window active
```

API trials:

```bash
npm run bench:discovery:api -- --profile smoke --manifest benchmarks/discovery/out/tiny-mixed-active/manifest.json
```

UI trials:

```bash
npm run bench:discovery:ui
```

Report:

```bash
npm run bench:discovery:report
```

Full 10k and 100k fixtures:

```bash
npm run bench:discovery:data -- generate --all-full
```

Then rebuild historical blocks for every `*-historical` and `*-mixed`
directory under `benchmarks/discovery/out/`.

## Cache classes

Report these separately. A Docker restart on macOS is process-cold, not
disk-cold, because Docker Desktop can keep page cache.

- `browser_cold`: new Playwright context.
- `client_warm`: same page, identical request.
- `process_cold`: new Prometheus process with a copied dataset volume.
- `storage_warm`: five identical requests before measurement.
- `cluster_cache_cold`: unique blocks or an explicit cache flush.

## Gates

The report fails when Search misses these product-level gates:

- Selective queries with at most 1,000 expected results have 100% recall in
  the judged set.
- Exact expected results rank first.
- Search nDCG@10 stays within 0.02 of Labels plus client-side ranking.
- Broad 100k empty searches at limit 10,000 set `has_more=true`.
- Warm Search first parsed batch is at most 35% of Labels completion.
- Search p95 completion is no more than 1.25 times Labels p95.
- Selective Search decoded payload is at most half the Labels payload.
- Client cancel returns within 250 ms.
- Error rate stays below 1%.

## Tests

```bash
go test -C benchmarks/discovery ./...
```
