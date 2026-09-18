import { css } from '@emotion/css';
import { debounce } from 'lodash';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { type GrafanaTheme2, type SelectableValue, type TimeRange } from '@grafana/data';
import { selectors } from '@grafana/e2e-selectors';
import { Trans, t } from '@grafana/i18n';
import { EditorField, EditorFieldGroup } from '@grafana/plugin-ui';
import { reportInteraction } from '@grafana/runtime';
import { Button, InlineField, InlineFieldRow, Select, useTheme2 } from '@grafana/ui';

import { DEFAULT_COMPLETION_LIMIT, METRIC_LABEL } from '../../constants';
import { type PrometheusDatasource } from '../../datasource';
import { SearchApiUnavailableError } from '../../search_api_stream';
import { type QueryBuilderLabelFilter } from '../shared/types';
import { type PromVisualQuery } from '../types';

import { formatKeyValueStrings, formatLabelFiltersToString } from './formatter';
import { MetricsModal } from './metrics-modal/MetricsModal';

export interface MetricComboboxProps {
  metricLookupDisabled: boolean;
  query: PromVisualQuery;
  onChange: (query: PromVisualQuery) => void;
  onGetMetrics: () => Promise<SelectableValue[]>;
  datasource: PrometheusDatasource;
  labelsFilters: QueryBuilderLabelFilter[];
  onBlur?: () => void;
  variableEditor?: boolean;
  timeRange: TimeRange;
}

export function MetricCombobox({
  datasource,
  query,
  onChange,
  onGetMetrics,
  labelsFilters,
  variableEditor,
  timeRange,
}: Readonly<MetricComboboxProps>) {
  const [metricsModalOpen, setMetricsModalOpen] = useState(false);
  const [metricOptions, setMetricOptions] = useState<Array<SelectableValue<string>>>([]);
  const [metricInput, setMetricInput] = useState('');
  const [isLoadingOptions, setIsLoadingOptions] = useState(false);
  const searchAbortControllerRef = useRef<AbortController>();
  const latestSearchIdRef = useRef(0);
  const styles = getStyles(useTheme2());

  const loadMetricOptions = useCallback(
    async (input: string, searchId: number) => {
      if (searchId !== latestSearchIdRef.current) {
        return;
      }

      setIsLoadingOptions(true);
      setMetricOptions([]);

      if (!input.length) {
        const metrics = await onGetMetrics();
        if (searchId === latestSearchIdRef.current) {
          setMetricOptions(
            metrics.map((option) => ({
              label: option.label ?? option.value,
              value: option.value,
            }))
          );
        }
        return;
      }

      const searchClient = datasource.languageProvider.getSearchApiClient?.();
      if (searchClient) {
        const abortController = new AbortController();
        searchAbortControllerRef.current = abortController;
        const rawMatch = formatLabelFiltersToString(labelsFilters) || undefined;
        const match = rawMatch ? datasource.interpolateString(rawMatch) : undefined;
        let streamedOptions: Array<SelectableValue<string>> = [];
        const publishBatch = (batch: Array<{ name: string }>) => {
          if (searchId !== latestSearchIdRef.current) {
            return;
          }
          const remaining = DEFAULT_COMPLETION_LIMIT - streamedOptions.length;
          const nextOptions = batch.slice(0, Math.max(0, remaining)).map((result) => ({
            label: result.name,
            value: result.name,
          }));
          streamedOptions = [...streamedOptions, ...nextOptions];
          setMetricOptions(streamedOptions);
        };

        try {
          const response = await searchClient.searchMetricNames(timeRange, input, {
            limit: DEFAULT_COMPLETION_LIMIT,
            match,
            onBatch: publishBatch,
            retainResults: false,
            signal: abortController.signal,
          });
          if (streamedOptions.length === 0) {
            publishBatch(response.results);
          }
          return;
        } catch (error) {
          if (!(error instanceof SearchApiUnavailableError)) {
            throw error;
          }
        }
      }

      const match = formatKeyValueStrings(input, labelsFilters);
      const results = await datasource.languageProvider.queryLabelValues(timeRange, METRIC_LABEL, match);

      if (searchId === latestSearchIdRef.current) {
        setMetricOptions(
          results.map((result) => ({
            label: result,
            value: result,
          }))
        );
      }
    },
    [datasource, labelsFilters, onGetMetrics, timeRange]
  );

  const debouncedLoadMetricOptions = useMemo(
    () =>
      debounce((input: string, searchId: number) => {
        void loadMetricOptions(input, searchId)
          .catch((error) => {
            if (!(error instanceof Error && error.name === 'AbortError')) {
              console.warn('Failed to query metric names:', error);
            }
          })
          .finally(() => {
            if (searchId === latestSearchIdRef.current) {
              setIsLoadingOptions(false);
            }
          });
      }, 200),
    [loadMetricOptions]
  );

  const requestMetricOptions = useCallback(
    (input: string, immediate = false) => {
      const searchId = ++latestSearchIdRef.current;
      searchAbortControllerRef.current?.abort();
      setIsLoadingOptions(true);
      if (immediate) {
        void loadMetricOptions(input, searchId)
          .catch((error) => {
            if (!(error instanceof Error && error.name === 'AbortError')) {
              console.warn('Failed to query metric names:', error);
            }
          })
          .finally(() => {
            if (searchId === latestSearchIdRef.current) {
              setIsLoadingOptions(false);
            }
          });
      } else {
        debouncedLoadMetricOptions(input, searchId);
      }
    },
    [debouncedLoadMetricOptions, loadMetricOptions]
  );

  const closeMetricOptions = useCallback(() => {
    latestSearchIdRef.current++;
    searchAbortControllerRef.current?.abort();
    debouncedLoadMetricOptions.cancel();
    setIsLoadingOptions(false);
  }, [debouncedLoadMetricOptions]);

  useEffect(
    () => () => {
      latestSearchIdRef.current++;
      searchAbortControllerRef.current?.abort();
      debouncedLoadMetricOptions.cancel();
    },
    [debouncedLoadMetricOptions]
  );

  const onComboboxChange = useCallback(
    (opt: SelectableValue<string> | null) => {
      setMetricInput('');
      onChange({ ...query, metric: opt?.value ?? '' });
    },
    [onChange, query]
  );

  const onMetricInputChange = useCallback(
    (input: string, actionMeta: { action: string }) => {
      if (actionMeta.action === 'input-change') {
        setMetricInput(input);
        requestMetricOptions(input);
      }
    },
    [requestMetricOptions]
  );

  const asyncSelect = () => {
    return (
      <div className={styles.wrapper}>
        <Select<string>
          aria-label={t(
            'grafana-prometheus.querybuilder.metric-combobox.async-select.placeholder-select-metric',
            'Select metric'
          )}
          placeholder={t(
            'grafana-prometheus.querybuilder.metric-combobox.async-select.placeholder-select-metric',
            'Select metric'
          )}
          width="auto"
          options={metricOptions}
          inputValue={metricInput}
          isLoading={isLoadingOptions}
          value={query.metric ? { label: query.metric, value: query.metric } : null}
          onChange={onComboboxChange}
          onInputChange={onMetricInputChange}
          filterOption={() => true}
          onOpenMenu={() => requestMetricOptions(metricInput, true)}
          onCloseMenu={closeMetricOptions}
          allowCustomValue
          allowCreateWhileLoading
          onCreateOption={(value) => onChange({ ...query, metric: value })}
          data-testid={selectors.components.DataSource.Prometheus.queryEditor.builder.metricSelect}
        />
        <Button
          tooltip={t(
            'grafana-prometheus.querybuilder.metric-combobox.async-select.tooltip-open-metrics-explorer',
            'Open metrics explorer'
          )}
          aria-label={t(
            'grafana-prometheus.querybuilder.metric-combobox.async-select.aria-label-open-metrics-explorer',
            'Open metrics explorer'
          )}
          variant="secondary"
          icon="book-open"
          className={styles.button}
          disabled={datasource.lookupsDisabled}
          onClick={() => {
            reportInteraction('grafana_prometheus_metrics_explorer_opened', {
              hasSelectedMetric: !!query.metric,
            });
            setMetricsModalOpen(true);
          }}
        />
      </div>
    );
  };

  return (
    <>
      {!datasource.lookupsDisabled && metricsModalOpen && (
        <MetricsModal
          datasource={datasource}
          isOpen={metricsModalOpen}
          onClose={() => setMetricsModalOpen(false)}
          query={query}
          onChange={onChange}
          timeRange={timeRange}
        />
      )}
      {variableEditor ? (
        <InlineFieldRow>
          <InlineField
            label={t('grafana-prometheus.querybuilder.metric-combobox.label-metric', 'Metric')}
            labelWidth={20}
            tooltip={
              <div>
                <Trans i18nKey="grafana-prometheus.querybuilder.metric-combobox.tooltip-metric">
                  Optional: returns a list of label values for the label name in the specified metric.
                </Trans>
              </div>
            }
          >
            {asyncSelect()}
          </InlineField>
        </InlineFieldRow>
      ) : (
        <EditorFieldGroup>
          <EditorField label={t('grafana-prometheus.querybuilder.metric-combobox.label-metric', 'Metric')}>
            {asyncSelect()}
          </EditorField>
        </EditorFieldGroup>
      )}
    </>
  );
}

const getStyles = (theme: GrafanaTheme2) => {
  return {
    wrapper: css({
      display: 'flex',
      input: {
        borderTopRightRadius: 'unset',
        borderBottomRightRadius: 'unset',
      },
    }),
    button: css({
      borderTopLeftRadius: 'unset',
      borderBottomLeftRadius: 'unset',
    }),
  };
};
