// Core grafana history https://github.com/grafana/grafana/blob/v11.0.0-preview/public/app/plugins/datasource/prometheus/components/monaco-query-field/monaco-completion-provider/completions.ts
import { type languages } from 'monaco-editor';

import { type TimeRange } from '@grafana/data';
import { config } from '@grafana/runtime';

import { DEFAULT_COMPLETION_LIMIT } from '../../../constants';
import { escapeLabelValueInExactSelector, prometheusRegularEscape } from '../../../escaping';
import { getFunctions } from '../../../promql';
import { rangeModifierDocumentation } from '../../../rangeModifiers';
import { isValidLegacyName } from '../../../utf8_support';

import { type DataProvider } from './data_provider';
import { type TriggerType } from './monaco-completion-provider';
import type { Label, Situation } from './situation';
import { NeverCaseError } from './util';
// FIXME: we should not load this from the "outside", but we cannot do that while we have the "old" query-field too

export type CompletionType =
  | 'HISTORY'
  | 'FUNCTION'
  | 'METRIC_NAME'
  | 'DURATION'
  | 'LABEL_NAME'
  | 'LABEL_VALUE'
  | 'KEYWORD';

// We cannot use languages.CompletionItemInsertTextRule.InsertAsSnippet because grafana-prometheus package isn't compatible
// It should first change the moduleResolution to bundler for TS to correctly resolve the types
// https://github.com/grafana/grafana/pull/96450
const InsertAsSnippet = 4;

export type Completion = {
  type: CompletionType;
  label: string;
  insertText: string;
  insertTextRules?: languages.CompletionItemInsertTextRule;
  detail?: string;
  documentation?: string;
  triggerOnInsert?: boolean;
};

// Snippet Marker is  telling monaco where to show the cursor and maybe a help text
// With help text example: ${1:labelName}
// labelName will be shown as selected. So user would know what to type next
const snippetMarker = '${1:}';

// Maximum number of recent queries surfaced as history completions.
const MAX_HISTORY_COMPLETIONS = 10;

// we order items like: history, functions, metrics
async function getAllMetricNamesCompletions(
  searchTerm: string | undefined,
  dataProvider: DataProvider,
  timeRange: TimeRange,
  onProgress?: (completions: Completion[]) => void
): Promise<Completion[]> {
  const toCompletions = (metricNames: string[]): Completion[] =>
    dataProvider.metricNamesToMetrics(metricNames).map((metric) => ({
      type: 'METRIC_NAME',
      label: metric.name,
      detail: `${metric.name} : ${metric.type}`,
      documentation: metric.help,
      ...(metric.isUtf8
        ? {
            insertText: `{"${metric.name}"${snippetMarker}}`,
            insertTextRules: InsertAsSnippet,
          }
        : {
            insertText: metric.name,
          }),
    }));
  const metricNames = onProgress
    ? await dataProvider.queryMetricNames(timeRange, searchTerm, (names) => onProgress(toCompletions(names)))
    : await dataProvider.queryMetricNames(timeRange, searchTerm);

  return toCompletions(metricNames);
}

const getFunctionCompletions: () => Completion[] = () => {
  return getFunctions().map((f) => ({
    type: 'FUNCTION',
    label: f.label,
    insertText: f.insertText ?? '', // i don't know what to do when this is nullish. it should not be.
    detail: f.detail,
    documentation: f.documentation,
  }));
};

async function getFunctionsOnlyCompletions(): Promise<Completion[]> {
  return Promise.resolve(getFunctionCompletions());
}

async function getAllFunctionsAndMetricNamesCompletions(
  searchTerm: string | undefined,
  dataProvider: DataProvider,
  timeRange: TimeRange,
  onProgress?: (completions: Completion[]) => void
): Promise<Completion[]> {
  const functions = getFunctionCompletions();
  const metricNames = onProgress
    ? await getAllMetricNamesCompletions(searchTerm, dataProvider, timeRange, (completions) =>
        onProgress([...functions, ...completions])
      )
    : await getAllMetricNamesCompletions(searchTerm, dataProvider, timeRange);
  return [...functions, ...metricNames];
}

const DURATION_COMPLETIONS: Completion[] = [
  '$__interval',
  '$__range',
  '$__rate_interval',
  '1m',
  '5m',
  '10m',
  '30m',
  '1h',
  '1d',
].map((text) => ({
  type: 'DURATION',
  label: text,
  insertText: text,
}));

function getAllHistoryCompletions(dataProvider: DataProvider): Completion[] {
  // function getAllHistoryCompletions(queryHistory: PromHistoryItem[]): Completion[] {
  // NOTE: the typescript types are wrong. historyItem.query.expr can be undefined
  const allHistory = dataProvider.getHistory();
  // FIXME: find a better history-limit
  return allHistory.slice(0, MAX_HISTORY_COMPLETIONS).map((expr) => ({
    type: 'HISTORY',
    label: expr,
    insertText: expr,
  }));
}

function makeSelector(metricName: string | undefined, labels: Label[]): string | undefined {
  if (metricName === undefined && labels.length === 0) {
    return undefined;
  }

  const allLabels = [...labels];

  // we transform the metricName to a label, if it exists
  if (metricName !== undefined) {
    allLabels.push({ name: '__name__', value: metricName, op: '=' });
  }

  const allLabelTexts = allLabels.map(
    (label) => `${label.name}${label.op}"${escapeLabelValueInExactSelector(label.value)}"`
  );

  return `{${allLabelTexts.join(',')}}`;
}

async function getLabelNames(
  metric: string | undefined,
  otherLabels: Label[],
  dataProvider: DataProvider,
  timeRange: TimeRange,
  searchTerm?: string,
  onProgress?: (labelNames: string[]) => void
): Promise<string[]> {
  const selector = makeSelector(metric, otherLabels);
  const usedLabelNames = new Set([...otherLabels.map((label) => label.name), '__name__']);
  const filterUnusedNames = (labelNames: string[]) => labelNames.filter((name) => !usedLabelNames.has(name));
  const labelNames = onProgress
    ? await dataProvider.queryLabelKeys(timeRange, selector, DEFAULT_COMPLETION_LIMIT, searchTerm, (names) =>
        onProgress(filterUnusedNames(names))
      )
    : await dataProvider.queryLabelKeys(timeRange, selector, DEFAULT_COMPLETION_LIMIT, searchTerm);
  return filterUnusedNames(labelNames);
}

async function getLabelNamesForCompletions(
  metric: string | undefined,
  suffix: string,
  triggerOnInsert: boolean,
  otherLabels: Label[],
  dataProvider: DataProvider,
  timeRange: TimeRange,
  searchTerm?: string,
  onProgress?: (completions: Completion[]) => void
): Promise<Completion[]> {
  const toCompletions = (labelNames: string[]): Completion[] =>
    labelNames.map((text) => {
      const isUtf8 = !isValidLegacyName(text);
      return {
        type: 'LABEL_NAME',
        label: text,
        ...(isUtf8
          ? {
              insertText: `"${text}"${suffix}`,
              insertTextRules: InsertAsSnippet,
            }
          : {
              insertText: `${text}${suffix}`,
            }),
        triggerOnInsert,
      };
    });
  const labelNames = onProgress
    ? await getLabelNames(metric, otherLabels, dataProvider, timeRange, searchTerm, (names) =>
        onProgress(toCompletions(names))
      )
    : await getLabelNames(metric, otherLabels, dataProvider, timeRange, searchTerm);
  return toCompletions(labelNames);
}

async function getLabelNamesForSelectorCompletions(
  metric: string | undefined,
  otherLabels: Label[],
  dataProvider: DataProvider,
  timeRange: TimeRange,
  searchTerm?: string,
  onProgress?: (completions: Completion[]) => void
): Promise<Completion[]> {
  return getLabelNamesForCompletions(metric, '=', true, otherLabels, dataProvider, timeRange, searchTerm, onProgress);
}

async function getLabelNamesForByCompletions(
  metric: string | undefined,
  otherLabels: Label[],
  dataProvider: DataProvider,
  timeRange: TimeRange,
  searchTerm?: string,
  onProgress?: (completions: Completion[]) => void
): Promise<Completion[]> {
  return getLabelNamesForCompletions(metric, '', false, otherLabels, dataProvider, timeRange, searchTerm, onProgress);
}

async function getLabelValues(
  metric: string | undefined,
  labelName: string,
  otherLabels: Label[],
  dataProvider: DataProvider,
  timeRange: TimeRange,
  searchTerm?: string,
  onProgress?: (values: string[]) => void
): Promise<string[]> {
  const selector = makeSelector(metric, otherLabels);
  return onProgress
    ? await dataProvider.queryLabelValues(
        timeRange,
        labelName,
        selector,
        DEFAULT_COMPLETION_LIMIT,
        searchTerm,
        onProgress
      )
    : await dataProvider.queryLabelValues(timeRange, labelName, selector, DEFAULT_COMPLETION_LIMIT, searchTerm);
}

async function getLabelValuesForMetricCompletions(
  metric: string | undefined,
  labelName: string,
  betweenQuotes: boolean,
  otherLabels: Label[],
  dataProvider: DataProvider,
  timeRange: TimeRange,
  searchTerm?: string,
  onProgress?: (completions: Completion[]) => void
): Promise<Completion[]> {
  const toCompletions = (values: string[]): Completion[] =>
    values.map((text) => ({
      type: 'LABEL_VALUE',
      label: text,
      insertText: formatLabelValueForCompletion(text, betweenQuotes),
    }));
  const values = onProgress
    ? await getLabelValues(metric, labelName, otherLabels, dataProvider, timeRange, searchTerm, (results) =>
        onProgress(toCompletions(results))
      )
    : await getLabelValues(metric, labelName, otherLabels, dataProvider, timeRange, searchTerm);
  return toCompletions(values);
}

function formatLabelValueForCompletion(value: string, betweenQuotes: boolean): string {
  const text = config.featureToggles.prometheusSpecialCharsInLabelValues ? prometheusRegularEscape(value) : value;
  return betweenQuotes ? text : `"${text}"`;
}

export async function getCompletions(
  situation: Situation,
  dataProvider: DataProvider,
  timeRange: TimeRange,
  searchTerm?: string,
  triggerType: TriggerType = 'full',
  onProgress?: (completions: Completion[]) => void
): Promise<Completion[]> {
  switch (situation.type) {
    case 'RANGE_MODIFIER':
      return situation.modifiers.map((modifier) => ({
        type: 'KEYWORD',
        label: modifier,
        insertText: modifier,
        documentation: rangeModifierDocumentation[modifier],
      }));
    case 'IN_DURATION':
      return Promise.resolve(DURATION_COMPLETIONS);
    case 'IN_FUNCTION':
      return triggerType === 'full'
        ? getAllFunctionsAndMetricNamesCompletions(searchTerm, dataProvider, timeRange, onProgress)
        : getFunctionsOnlyCompletions();
    case 'AT_ROOT': {
      return triggerType === 'full'
        ? getAllFunctionsAndMetricNamesCompletions(searchTerm, dataProvider, timeRange, onProgress)
        : getFunctionsOnlyCompletions();
    }
    case 'EMPTY': {
      if (triggerType === 'partial') {
        return Promise.resolve(getFunctionCompletions());
      }
      const historyCompletions = getAllHistoryCompletions(dataProvider);
      const staticCompletions = [...historyCompletions, ...getFunctionCompletions()];
      const metricNames = onProgress
        ? await getAllMetricNamesCompletions(searchTerm, dataProvider, timeRange, (completions) =>
            onProgress([...staticCompletions, ...completions])
          )
        : await getAllMetricNamesCompletions(searchTerm, dataProvider, timeRange);
      return Promise.resolve([...staticCompletions, ...metricNames]);
    }
    case 'IN_LABEL_SELECTOR_NO_LABEL_NAME':
      return getLabelNamesForSelectorCompletions(
        situation.metricName,
        situation.otherLabels,
        dataProvider,
        timeRange,
        searchTerm,
        onProgress
      );
    case 'IN_GROUPING':
      return getLabelNamesForByCompletions(
        situation.metricName,
        situation.otherLabels,
        dataProvider,
        timeRange,
        searchTerm,
        onProgress
      );
    case 'IN_LABEL_SELECTOR_WITH_LABEL_NAME':
      return getLabelValuesForMetricCompletions(
        situation.metricName,
        situation.labelName,
        situation.betweenQuotes,
        situation.otherLabels,
        dataProvider,
        timeRange,
        searchTerm,
        onProgress
      );
    default:
      throw new NeverCaseError(situation);
  }
}
