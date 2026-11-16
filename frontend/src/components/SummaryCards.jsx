// The four summary figures of the dashboard. Presentational only: it shows
// the backend figures it is given, formatted for vi-VN, and computes nothing.

const numberFormatter = new Intl.NumberFormat('vi-VN')

// tone picks the accent colour: the low and out of stock cards use the same
// tokens as the stock badges of the product table and the status pie.
const CARDS = [
  { field: 'total_products', label: 'Tổng sản phẩm', tone: 'primary' },
  { field: 'total_stock', label: 'Tổng tồn kho', tone: 'accent' },
  { field: 'low_stock_products', label: 'Sản phẩm sắp hết hàng', tone: 'warning' },
  { field: 'out_of_stock_products', label: 'Sản phẩm hết hàng', tone: 'danger' },
]

export function SummaryCards({ summary }) {
  return (
    <dl className="summary-cards">
      {CARDS.map(({ field, label, tone }) => (
        <div key={field} className={`summary-card summary-card--${tone}`} data-field={field}>
          <dt className="summary-card-label">{label}</dt>
          <dd className="summary-card-value">{numberFormatter.format(summary[field])}</dd>
        </div>
      ))}
    </dl>
  )
}
