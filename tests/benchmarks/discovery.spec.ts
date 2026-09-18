// Metrics Explorer Search vs Labels discovery benchmark.
// Skipped in CI. Run with: npm run bench:discovery:ui

import { expect, test } from '@grafana/plugin-e2e';
import { type Page } from '@playwright/test';
import { writeFileSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';

const enabled = process.env.BENCH_DISCOVERY === '1';
const searchUid = process.env.BENCH_SEARCH_DATASOURCE || 'prometheus-search-api';
const labelsUid = process.env.BENCH_LABELS_DATASOURCE || 'prometheus-direct';
const outPath = process.env.BENCH_UI_OUT || resolve('benchmarks/discovery/out/ui-trials.jsonl');

const networks: Record<string, { latency: number; download: number; upload: number } | null> = {
  local: null,
  slow4g: { latency: 20, download: (1.6 * 1024 * 1024) / 8, upload: (768 * 1024) / 8 },
  fast3g: { latency: 562.5, download: (1.6 * 1024 * 1024) / 8, upload: (750 * 1024) / 8 },
  slow3g: { latency: 400, download: (400 * 1024) / 8, upload: (400 * 1024) / 8 },
};

const queries = [
  { kind: 'empty', term: '' },
  { kind: 'exact', term: 'http_requests_total' },
  { kind: 'multi-word', term: 'http req' },
  { kind: 'typo', term: 'promethues' },
  { kind: 'no-match', term: 'zzzz_no_such_metric_zzzz' },
];

type Trial = {
  pair_id: string;
  api: string;
  endpoint: string;
  query_kind: string;
  term: string;
  network: string;
  cache_class: string;
  warmup: boolean;
  first_byte_ns: number;
  first_batch_ns: number;
  first_render_ns: number;
  complete_ns: number;
  heap_bytes: number;
  result_count: number;
  error?: string;
};

test.describe('Discovery benchmark', () => {
  test.skip(!enabled, 'Set BENCH_DISCOVERY=1 to run discovery UI benchmarks');

  test('metrics explorer first render and cancellation', { tag: '@bench-discovery' }, async ({ page, explorePage }) => {
    const networkName = process.env.BENCH_NETWORK || 'local';
    await applyNetwork(page, networkName);

    const trials: Trial[] = [];
    const datasources = [
      { api: 'search', uid: searchUid },
      { api: 'labels', uid: labelsUid },
    ];

    for (const ds of datasources) {
      await explorePage.goto();
      await explorePage.datasource.set(ds.uid);
      await page.getByRole('radio', { name: 'Builder' }).click();
      await page.getByRole('button', { name: 'Open metrics explorer' }).click();
      const search = page.getByTestId('search-metric');
      await expect(search).toBeVisible();

      for (const query of queries) {
        const trial = await measureSearch(page, ds.api, query.kind, query.term, networkName);
        trials.push(trial);
      }

      await search.fill('http');
      await page.waitForTimeout(50);
      await search.fill('http_requests_total');
      await page.waitForTimeout(500);
      const stale = page.locator('table tbody tr', { hasText: 'zzzz_no_such_metric_zzzz' });
      await expect(stale).toHaveCount(0);

      await page.keyboard.press('Escape');
    }

    mkdirSync(dirname(outPath), { recursive: true });
    writeFileSync(outPath, trials.map((trial) => JSON.stringify(trial)).join('\n') + '\n');
  });
});

async function measureSearch(page: Page, api: string, kind: string, term: string, network: string): Promise<Trial> {
  const search = page.getByTestId('search-metric');
  const started = nowNs();
  let firstByteNs = 0;
  const pending = page.waitForResponse(
    (response) => {
      const url = response.url();
      return url.includes('/api/v1/search/') || url.includes('/api/v1/label/') || url.includes('/api/v1/labels');
    },
    { timeout: 60_000 }
  );

  await search.fill(term);
  try {
    const response = await pending;
    firstByteNs = nowNs() - started;
    await response.finished();
  } catch (error) {
    return {
      pair_id: `${api}-${kind}`,
      api,
      endpoint: 'metric_names',
      query_kind: kind,
      term,
      network,
      cache_class: 'client_warm',
      warmup: false,
      first_byte_ns: firstByteNs,
      first_batch_ns: 0,
      first_render_ns: 0,
      complete_ns: nowNs() - started,
      heap_bytes: 0,
      result_count: 0,
      error: error instanceof Error ? error.message : String(error),
    };
  }

  const firstRow = page.locator('table tbody tr').first();
  let firstRenderNs = 0;
  try {
    await firstRow.waitFor({ state: 'visible', timeout: kind === 'no-match' ? 2_000 : 30_000 });
    await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    firstRenderNs = nowNs() - started;
  } catch {
    firstRenderNs = nowNs() - started;
  }

  const spinner = page.locator('[class*="loadingSpinner"]');
  if (await spinner.count()) {
    await expect(spinner).toBeHidden({ timeout: 60_000 }).catch(() => undefined);
  }
  const completeNs = nowNs() - started;
  const resultCount = await page.locator('table tbody tr').count();
  const heap = await page.evaluate(() => {
    const memory = (performance as Performance & { memory?: { usedJSHeapSize: number } }).memory;
    return memory?.usedJSHeapSize ?? 0;
  });

  return {
    pair_id: `${api}-${kind}`,
    api,
    endpoint: 'metric_names',
    query_kind: kind,
    term,
    network,
    cache_class: 'client_warm',
    warmup: false,
    first_byte_ns: firstByteNs,
    first_batch_ns: firstByteNs,
    first_render_ns: firstRenderNs,
    complete_ns: completeNs,
    heap_bytes: heap,
    result_count: resultCount,
  };
}

async function applyNetwork(page: Page, name: string) {
  const profile = networks[name];
  if (!profile) {
    return;
  }
  const session = await page.context().newCDPSession(page);
  await session.send('Network.emulateNetworkConditions', {
    offline: false,
    latency: profile.latency,
    downloadThroughput: profile.download,
    uploadThroughput: profile.upload,
  });
}

function nowNs(): number {
  return Number(process.hrtime.bigint());
}
