const currencyFormatter = new Intl.NumberFormat('vi-VN', {
  style: 'currency',
  currency: 'VND',
})

export function ProductTable({
  products = [],
  loading = false,
  sortBy = 'id',
  order = 'asc',
  onSort,
  page = 1,
  limit = 10,
  onPageChange,
  onLimitChange,
  onEdit,
  onDelete,
  onStockIn,
  onStockOut,
  isAdmin = false,
}) {
  const renderSortIndicator = (field) => {
    if (sortBy !== field) {
      return <span className="sort-icon sort-inactive" aria-hidden="true">↕</span>
    }
    return (
      <span className="sort-icon sort-active" aria-hidden="true">
        {order === 'asc' ? '▲' : '▼'}
      </span>
    )
  }

  const getAriaSort = (field) => {
    if (sortBy !== field) return 'none'
    return order === 'asc' ? 'ascending' : 'descending'
  }

  const renderStockBadge = (quantity) => {
    if (quantity <= 0) {
      return <span className="badge badge-danger">Hết hàng (0)</span>
    }
    if (quantity <= 5) {
      return <span className="badge badge-warning">Sắp hết ({quantity})</span>
    }
    return <span className="badge badge-success">Còn hàng ({quantity})</span>
  }

  const isNextDisabled = loading || products.length < limit
  const isPrevDisabled = loading || page <= 1

  return (
    <div className="table-card">
      <div className="table-wrapper">
        <table className="product-table">
          <thead>
            <tr>
              <th
                scope="col"
                className="th-sortable th-id"
                onClick={() => onSort('id')}
                aria-sort={getAriaSort('id')}
              >
                <span>ID</span> {renderSortIndicator('id')}
              </th>
              <th scope="col" className="th-sku">SKU</th>
              <th
                scope="col"
                className="th-sortable th-name"
                onClick={() => onSort('name')}
                aria-sort={getAriaSort('name')}
              >
                <span>Tên sản phẩm</span> {renderSortIndicator('name')}
              </th>
              <th
                scope="col"
                className="th-sortable th-price text-right"
                onClick={() => onSort('price')}
                aria-sort={getAriaSort('price')}
              >
                <span>Đơn giá</span> {renderSortIndicator('price')}
              </th>
              <th scope="col" className="th-quantity text-center">Tồn kho</th>
              <th scope="col" className="th-actions text-center">Thao tác</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr>
                <td colSpan="6" className="table-state-cell">
                  <div className="loading-indicator">
                    <span className="spinner" aria-hidden="true"></span>
                    <span>Đang tải danh sách sản phẩm...</span>
                  </div>
                </td>
              </tr>
            ) : products.length === 0 ? (
              <tr>
                <td colSpan="6" className="table-state-cell">
                  <div className="empty-indicator">
                    <p className="empty-title">Không tìm thấy sản phẩm</p>
                    <p className="empty-description">
                      Không có sản phẩm nào trên trang này hoặc phù hợp với bộ lọc hiện tại.
                    </p>
                  </div>
                </td>
              </tr>
            ) : (
              products.map((p) => (
                <tr key={p.id}>
                  <td className="td-id tabular-nums">{p.id}</td>
                  <td className="td-sku">
                    <span className="sku-code">{p.sku}</span>
                  </td>
                  <td className="td-name font-medium">{p.name}</td>
                  <td className="td-price tabular-nums text-right">
                    {currencyFormatter.format(p.price)}
                  </td>
                  <td className="td-quantity text-center">
                    {renderStockBadge(p.quantity)}
                  </td>
                  <td className="td-actions text-center">
                    <div className="actions-group">
                      {isAdmin && (
                        <button
                          type="button"
                          className="btn-action btn-edit"
                          onClick={() => onEdit?.(p)}
                          title={`Chỉnh sửa thông tin ${p.name}`}
                        >
                          Sửa
                        </button>
                      )}
                      <button
                        type="button"
                        className="btn-action btn-stock-in"
                        onClick={() => onStockIn?.(p)}
                        title={`Nhập thêm tồn kho cho ${p.name}`}
                      >
                        Nhập kho
                      </button>
                      <button
                        type="button"
                        className="btn-action btn-stock-out"
                        onClick={() => onStockOut?.(p)}
                        title={`Xuất bớt tồn kho cho ${p.name}`}
                      >
                        Xuất kho
                      </button>
                      {isAdmin && (
                        <button
                          type="button"
                          className="btn-action btn-delete"
                          onClick={() => onDelete?.(p)}
                          title={`Xóa sản phẩm ${p.name}`}
                        >
                          Xóa
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination Bar */}
      <div className="pagination-bar">
        <div className="pagination-info">
          <label htmlFor="limit-select" className="pagination-label">
            Hiển thị:
          </label>
          <select
            id="limit-select"
            className="limit-select"
            value={limit}
            onChange={(e) => onLimitChange(Number(e.target.value))}
            disabled={loading}
          >
            <option value={10}>10</option>
            <option value={20}>20</option>
            <option value={50}>50</option>
          </select>
          <span className="page-summary">
            • Trang {page} ({loading ? '...' : `${products.length} sản phẩm`})
          </span>
        </div>

        <div className="pagination-controls">
          <button
            type="button"
            className="btn btn-pagination"
            onClick={() => onPageChange(page - 1)}
            disabled={isPrevDisabled}
          >
            ← Trang trước
          </button>
          <span className="current-page-badge">Trang {page}</span>
          <button
            type="button"
            className="btn btn-pagination"
            onClick={() => onPageChange(page + 1)}
            disabled={isNextDisabled}
          >
            Trang sau →
          </button>
        </div>
      </div>
    </div>
  )
}
