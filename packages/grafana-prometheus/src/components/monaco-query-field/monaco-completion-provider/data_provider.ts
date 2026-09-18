import { type HistoryItem, type TimeRange } from '@grafana/data';

import { DEFAULT_COMPLETION_LIMIT, METRIC_LABEL, SEARCH_STREAM_BATCH_SIZE } from '../../../constants';
import { type PrometheusLanguageProviderInterface } from '../../../language_provider';
import { removeQuotesIfExist } from '../../../language_utils';
import { SearchApiUnavailableError } from '../../../search_api_stream';
import { type PromQuery } from '../../../types';
import { escapeForUtf8Support, isValidLegacyName } from '../../../utf8_support';

interface Metric {
  name: string;
  help: string;
  type: string;
  isUtf8?: boolean;
}

export interface DataProviderParams {
  languageProvider: PrometheusLanguageProviderInterface;
  historyProvider: Array<HistoryItem<PromQuery>>;
}

type ProgressiveResultsCallback = (results: string[]) => void;

export class DataProvider {
  readonly languageProvider: PrometheusLanguageProviderInterface;
  readonly historyProvider: Array<HistoryItem<PromQuery>>;

  private metricSearchAbortController?: AbortController;
  private labelKeySearchAbortController?: AbortController;
  private labelValueSearchAbortController?: AbortController;

  constructor(params: DataProviderParams) {
    this.languageProvider = params.languageProvider;
    this.historyProvider = params.historyProvider;

    // Ensure metadata is loaded for completions. The builder mode triggers this via its own
    // components, but the code editor does not, so we need to fetch it here if not already cached.
    const existingMetadata = this.languageProvider.retrieveMetricsMetadata();
    if (Object.keys(existingMetadata).length === 0) {
      this.languageProvider.queryMetricsMetadata();
    }
  }

  /**
   * Queries metric names with optional filtering.
   * Safely constructs regex patterns and handles errors.
   */
  queryMetricNames = async (
    timeRange: TimeRange,
    searchTerm: string | undefined,
    onProgress?: ProgressiveResultsCallback
  ): Promise<string[]> => {
    try {
      const searchClient = this.languageProvider.getSearchApiClient?.();
      if (searchClient) {
        this.metricSearchAbortController?.abort();
        this.metricSearchAbortController = new AbortController();
        const streamedResults: string[] = [];
        const publishBatch = (batch: Array<{ name: string }>) => {
          streamedResults.push(...batch.map((result) => result.name));
          onProgress?.(streamedResults.slice());
        };

        try {
          const response = await searchClient.searchMetricNames(timeRange, searchTerm ?? '', {
            limit: DEFAULT_COMPLETION_LIMIT,
            batchSize: monacoSearchBatchSize(searchTerm),
            onBatch: publishBatch,
            retainResults: false,
            signal: this.metricSearchAbortController.signal,
          });
          if (streamedResults.length === 0) {
            publishBatch(response.results);
          }
          return streamedResults;
        } catch (error) {
          if (!(error instanceof SearchApiUnavailableError)) {
            throw error;
          }
        }
      }

      let match: string | undefined;
      if (searchTerm) {
        const escapedWord = escapeForUtf8Support(removeQuotesIfExist(searchTerm));
        match = `{__name__=~".*${escapedWord}.*"}`;
      }

      const result = await this.languageProvider.queryLabelValues(
        timeRange,
        METRIC_LABEL,
        match,
        DEFAULT_COMPLETION_LIMIT
      );

      const metricNames = Array.isArray(result) ? result : [];
      onProgress?.(metricNames);
      return metricNames;
    } catch (error) {
      if (!isAbortError(error)) {
        console.warn('Failed to query metric names:', error);
      }
      return [];
    }
  };

  queryLabelKeys = async (
    timeRange: TimeRange,
    match?: string,
    limit?: number,
    searchTerm?: string,
    onProgress?: ProgressiveResultsCallback
  ): Promise<string[]> => {
    const searchClient = this.languageProvider.getSearchApiClient?.();
    if (searchClient && searchTerm) {
      this.labelKeySearchAbortController?.abort();
      this.labelKeySearchAbortController = new AbortController();
      const streamedResults: string[] = [];
      const publishBatch = (batch: Array<{ name: string }>) => {
        streamedResults.push(...batch.map((result) => result.name));
        onProgress?.(streamedResults.slice());
      };

      try {
        const response = await searchClient.searchLabelNames(timeRange, searchTerm, {
          limit: limit ?? DEFAULT_COMPLETION_LIMIT,
          match: match ? this.languageProvider.datasource.interpolateString(match) : undefined,
          onBatch: publishBatch,
          retainResults: false,
          signal: this.labelKeySearchAbortController.signal,
        });
        if (streamedResults.length === 0) {
          publishBatch(response.results);
        }
        return streamedResults;
      } catch (error) {
        if (!(error instanceof SearchApiUnavailableError)) {
          throw error;
        }
      }
    }

    const labelKeys = await this.languageProvider.queryLabelKeys(timeRange, match, limit);
    onProgress?.(labelKeys);
    return labelKeys;
  };

  queryLabelValues = async (
    timeRange: TimeRange,
    labelKey: string,
    match?: string,
    limit?: number,
    searchTerm?: string,
    onProgress?: ProgressiveResultsCallback
  ): Promise<string[]> => {
    const searchClient = this.languageProvider.getSearchApiClient?.();
    if (searchClient && searchTerm) {
      this.labelValueSearchAbortController?.abort();
      this.labelValueSearchAbortController = new AbortController();
      const streamedResults: string[] = [];
      const publishBatch = (batch: Array<{ value: string }>) => {
        streamedResults.push(...batch.map((result) => result.value));
        onProgress?.(streamedResults.slice());
      };

      try {
        const response = await searchClient.searchLabelValues(
          timeRange,
          removeQuotesIfExist(this.languageProvider.datasource.interpolateString(labelKey)),
          removeQuotesIfExist(searchTerm),
          {
            limit: limit ?? DEFAULT_COMPLETION_LIMIT,
            match: match ? this.languageProvider.datasource.interpolateString(match) : undefined,
            onBatch: publishBatch,
            retainResults: false,
            signal: this.labelValueSearchAbortController.signal,
          }
        );
        if (streamedResults.length === 0) {
          publishBatch(response.results);
        }
        return streamedResults;
      } catch (error) {
        if (!(error instanceof SearchApiUnavailableError)) {
          throw error;
        }
      }
    }

    const labelValues = await this.languageProvider.queryLabelValues(timeRange, labelKey, match, limit);
    onProgress?.(labelValues);
    return labelValues;
  };

  dispose(): void {
    this.metricSearchAbortController?.abort();
    this.labelKeySearchAbortController?.abort();
    this.labelValueSearchAbortController?.abort();
  }

  getHistory(): string[] {
    return this.historyProvider.map((h) => h.query.expr).filter(Boolean);
  }

  metricNamesToMetrics(metricNames: string[]): Metric[] {
    const metricsMetadata = this.languageProvider.retrieveMetricsMetadata();
    const result: Metric[] = metricNames.map((m) => {
      const metaItem = metricsMetadata?.[m];
      return {
        name: m,
        help: metaItem?.help ?? '',
        type: metaItem?.type ?? '',
        isUtf8: !isValidLegacyName(m),
      };
    });

    return result;
  }
}

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === 'AbortError';
}

// Unfiltered Monaco lists retrigger the native suggest widget per batch.
// Until a Grafana-owned overlay can append without remounting, request the
// full completion cap as one batch when there is no prefix to narrow on.
function monacoSearchBatchSize(searchTerm?: string): number {
  return searchTerm?.trim() ? SEARCH_STREAM_BATCH_SIZE : DEFAULT_COMPLETION_LIMIT;
}
