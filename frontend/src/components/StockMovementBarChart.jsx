import { useMemo } from 'react'
import { Bar, BarChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import {
  formatChartAxisLabel,
  formatChartGranularity,
  formatChartRange,
  formatChartTooltipLabel,
} from '../utils/dashboardChartLabels'

// Stock in / stock out bar chart of the dashboard. Presentational only: it
// draws the model built by toStockMovementChartModel (utils/dashboardChart)
// as two grouped bar series and computes nothing. Rows go to Recharts as
// they are (zero buckets included), the totals are the backend's, and every
// date label comes from utils/dashboardChartLabels.

const numberFormatter = new Intl.NumberFormat('vi-VN')

export const formatQuantity = (value) => numberFormatter.format(value)

// Series in legend order: stock in first, then stock out.
export const STOCK_CHART_SERIES = [
  { dataKey: 'stock_in', name: 'Nhập kho', color: 'var(--color-accent)' },
  { dataKey: 'stock_out', name: 'Xuất kho', color: 'var(--color-warning)' },
]

// Recharts sorts legend items alphabetically by default; keep the series
// order. Module level, so Recharts always gets the same function.
export function legendSeriesSorter(item) {
  const index = STOCK_CHART_SERIES.findIndex((s) => s.name === item.value)
  return index === -1 ? STOCK_CHART_SERIES.length : index
}

// X axis ticks and tooltip titles for the model's granularity.
export const axisTickFormatter = (granularity) => (period) => formatChartAxisLabel(period, granularity)
export const tooltipLabelFormatter = (granularity) => (period) => formatChartTooltipLabel(period, granularity)

// Tooltip line of a series: its name and the backend quantity.
export const tooltipValueFormatter = (value, name) => [formatQuantity(value), name]

export const EMPTY_CHART_MESSAGE = 'Không có dữ liệu cho khoảng thời gian này.'
export const ALL_ZERO_CHART_MESSAGE = 'Không có giao dịch nhập hoặc xuất kho trong khoảng thời gian này.'

// The figures as one sentence, for screen readers: range, granularity and
// the backend totals. No per-bucket table.
export function stockChartSummaryText(model) {
  const range = formatChartRange(model.from, model.to)
  const granularity = formatChartGranularity(model.granularity).toLowerCase()
  return `${range}, ${granularity}: tổng nhập kho ${formatQuantity(model.totals.stock_in)}, tổng xuất kho ${formatQuantity(model.totals.stock_out)}.`
}

const TITLE_ID = 'stock-movement-chart-title'

export function StockMovementBarChart({ model }) {
  const formatTick = useMemo(() => axisTickFormatter(model.granularity), [model.granularity])
  const formatTooltipLabel = useMemo(() => tooltipLabelFormatter(model.granularity), [model.granularity])

  return (
    <section className="stock-chart" aria-labelledby={TITLE_ID} data-state={model.state}>
      <div className="stock-chart-header">
        <h3 id={TITLE_ID} className="stock-chart-title">
          Nhập và xuất kho
        </h3>
        <p className="stock-chart-range">
          {formatChartRange(model.from, model.to)} · {formatChartGranularity(model.granularity)}
        </p>
      </div>

      {model.state === 'empty' ? (
        <p className="stock-chart-empty">{EMPTY_CHART_MESSAGE}</p>
      ) : (
        <>
          <dl className="stock-chart-totals">
            <div className="stock-chart-total stock-chart-total--in">
              <dt>Tổng nhập kho</dt>
              <dd>{formatQuantity(model.totals.stock_in)}</dd>
            </div>
            <div className="stock-chart-total stock-chart-total--out">
              <dt>Tổng xuất kho</dt>
              <dd>{formatQuantity(model.totals.stock_out)}</dd>
            </div>
          </dl>

          {model.state === 'all_zero' && <p className="stock-chart-note">{ALL_ZERO_CHART_MESSAGE}</p>}

          <p className="sr-only">{stockChartSummaryText(model)}</p>
          {/* The container has a CSS height: ResponsiveContainer needs one. */}
          <div className="stock-chart-canvas">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={model.rows} margin={{ top: 8, right: 8, bottom: 0, left: 0 }} barGap={2}>
                <CartesianGrid vertical={false} strokeDasharray="3 3" stroke="var(--color-border)" />
                <XAxis
                  dataKey="period"
                  tickFormatter={formatTick}
                  interval="preserveStartEnd"
                  minTickGap={12}
                  tick={{ fontSize: 12 }}
                />
                <YAxis allowDecimals={false} tickFormatter={formatQuantity} width="auto" tick={{ fontSize: 12 }} />
                <Tooltip
                  labelFormatter={formatTooltipLabel}
                  formatter={tooltipValueFormatter}
                  cursor={{ fill: 'var(--color-surface-muted)' }}
                />
                <Legend itemSorter={legendSeriesSorter} />
                {STOCK_CHART_SERIES.map((series) => (
                  <Bar key={series.dataKey} dataKey={series.dataKey} name={series.name} fill={series.color} maxBarSize={28} />
                ))}
              </BarChart>
            </ResponsiveContainer>
          </div>
        </>
      )}
    </section>
  )
}
