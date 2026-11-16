import { useMemo } from 'react'
import { Cell, Legend, Pie, PieChart, ResponsiveContainer, Tooltip } from 'recharts'

// Inventory status pie of the dashboard. Presentational only: it draws the
// model built by toInventoryStatusPieModel (utils/dashboardInventoryStatus)
// and computes nothing. Every slice is passed to Recharts, including those
// whose count is 0; the colours are the model's CSS variables.

const numberFormatter = new Intl.NumberFormat('vi-VN')

export const formatProductCount = (count) => `${numberFormatter.format(count)} sản phẩm`

// Legend entry of a slice (found by its label): the label and its backend
// count, so the legend does not rely on colour alone.
export function legendText(model, label) {
  const slice = model.slices.find((s) => s.label === label)
  return slice ? `${slice.label}: ${numberFormatter.format(slice.count)}` : label
}

// Recharts sorts legend items alphabetically by default; this keeps the
// backend order (normal, low_stock, out_of_stock). Items are matched by
// label; an unknown one goes last.
export function legendItemSorter(model) {
  return (item) => {
    const index = model.slices.findIndex((s) => s.label === item.value)
    return index === -1 ? model.slices.length : index
  }
}

// Tooltip line of a slice: its Vietnamese label and its backend count.
export const tooltipFormatter = (value, name) => [formatProductCount(value), name]

// The figures as one sentence, for screen readers.
export function statusSummaryText(model) {
  const parts = model.slices.map((s) => `${s.label}: ${numberFormatter.format(s.count)}`)
  return `Tổng ${formatProductCount(model.total_products)}. ${parts.join(', ')}.`
}

const TITLE_ID = 'inventory-status-pie-title'

export function InventoryStatusPie({ model }) {
  // Stable per model: Recharts stores the sorter and re-dispatches on change.
  const itemSorter = useMemo(() => legendItemSorter(model), [model])

  return (
    <section className="status-pie" aria-labelledby={TITLE_ID} data-state={model.state}>
      <div className="status-pie-header">
        <h3 id={TITLE_ID} className="status-pie-title">
          Phân bố trạng thái tồn kho
        </h3>
        <p className="status-pie-total">Tổng: {formatProductCount(model.total_products)}</p>
      </div>

      {model.state === 'empty' ? (
        <p className="status-pie-empty">Chưa có sản phẩm nào để thống kê trạng thái tồn kho.</p>
      ) : (
        <>
          <p className="sr-only">{statusSummaryText(model)}</p>
          {/* The container has a CSS height: ResponsiveContainer needs one. */}
          <div className="status-pie-chart">
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={model.slices}
                  dataKey="count"
                  nameKey="label"
                  innerRadius="55%"
                  outerRadius="80%"
                  stroke="var(--color-surface)"
                  strokeWidth={2}
                >
                  {model.slices.map((slice) => (
                    <Cell key={slice.status} fill={slice.color} />
                  ))}
                </Pie>
                <Tooltip formatter={tooltipFormatter} />
                <Legend formatter={(value) => legendText(model, value)} itemSorter={itemSorter} />
              </PieChart>
            </ResponsiveContainer>
          </div>
        </>
      )}
    </section>
  )
}
