import { useState, useEffect, useCallback } from 'react'
import { getProducts } from './services/api'
import { useAuth } from './contexts/AuthContext'
import { LoginView } from './components/LoginView'
import { SearchBar } from './components/SearchBar'
import { ProductTable } from './components/ProductTable'
import { CreateProductModal } from './components/CreateProductModal'
import { EditProductModal } from './components/EditProductModal'
import { DeleteConfirmModal } from './components/DeleteConfirmModal'
import { StockModal } from './components/StockModal'
import './App.css'

export default function App() {
  const { user, loading: authLoading, logout } = useAuth()
  const [loggingOut, setLoggingOut] = useState(false)

  const [products, setProducts] = useState([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState(null)
  const [successMessage, setSuccessMessage] = useState(null)
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false)
  const [editingProductId, setEditingProductId] = useState(null)
  const [deletingProduct, setDeletingProduct] = useState(null)
  const [stockModalState, setStockModalState] = useState({
    isOpen: false,
    product: null,
    type: 'in',
  })

  const [searchQuery, setSearchQuery] = useState('')
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(10)
  const [sortBy, setSortBy] = useState('id')
  const [order, setOrder] = useState('asc')

  const fetchProducts = useCallback(async (isManualRefresh = false) => {
    if (isManualRefresh) {
      setRefreshing(true)
    } else {
      setLoading(true)
    }
    setError(null)

    try {
      const data = await getProducts({
        q: searchQuery,
        page,
        limit,
        sortBy,
        order,
      })
      setProducts(Array.isArray(data) ? data : [])
    } catch (err) {
      setError(err?.message || 'Không thể tải danh sách sản phẩm từ máy chủ.')
      setProducts([])
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [searchQuery, page, limit, sortBy, order])

  useEffect(() => {
    if (user) {
      fetchProducts()
    }
  }, [fetchProducts, user])

  // Auto-dismiss success notification after 5 seconds
  useEffect(() => {
    if (!successMessage) return
    const timer = setTimeout(() => {
      setSuccessMessage(null)
    }, 5000)
    return () => clearTimeout(timer)
  }, [successMessage])

  const handleSearchChange = (q) => {
    setSearchQuery(q)
    setPage(1)
  }

  const handleSort = (field) => {
    if (sortBy === field) {
      setOrder((prev) => (prev === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortBy(field)
      setOrder('asc')
    }
    setPage(1)
  }

  const handleLimitChange = (newLimit) => {
    setLimit(newLimit)
    setPage(1)
  }

  const handleRefresh = () => {
    fetchProducts(true)
  }

  const handleProductCreated = (newProduct) => {
    setSuccessMessage(`Đã thêm sản phẩm thành công: "${newProduct.name}" (SKU: ${newProduct.sku})`)
    // Reload list keeping current search/sort/page
    fetchProducts()
  }

  const handleProductUpdated = (updatedProduct) => {
    setSuccessMessage(`Đã cập nhật sản phẩm thành công: "${updatedProduct.name}" (SKU: ${updatedProduct.sku})`)
    // Reload list keeping current search/sort/page
    fetchProducts()
  }

  const handleProductDeleted = (deletedProduct) => {
    setSuccessMessage(`Đã xóa thành công sản phẩm: "${deletedProduct.name}" (SKU: ${deletedProduct.sku})`)
    // If the only item on current page was deleted and page > 1, go back one page
    if (products.length === 1 && page > 1) {
      setPage((prev) => prev - 1)
    } else {
      fetchProducts()
    }
  }

  const handleStockInClick = (product) => {
    setStockModalState({ isOpen: true, product, type: 'in' })
  }

  const handleStockOutClick = (product) => {
    setStockModalState({ isOpen: true, product, type: 'out' })
  }

  const handleStockSuccess = (updatedProduct, type, qty) => {
    const actionText = type === 'in' ? 'Nhập kho' : 'Xuất kho'
    setSuccessMessage(
      `Đã ${actionText.toLowerCase()} thành công ${qty} sản phẩm cho "${updatedProduct.name}" (Tồn kho mới: ${updatedProduct.quantity})`
    )
    fetchProducts()
  }

  const handleLogout = async () => {
    try {
      setLoggingOut(true)
      await logout()
    } catch (err) {
      console.error('Logout failed:', err)
    } finally {
      setLoggingOut(false)
    }
  }

  if (authLoading) {
    return (
      <div className="auth-loading-screen">
        <span className="spinner" aria-hidden="true"></span>
        <span>Đang kiểm tra phiên đăng nhập...</span>
      </div>
    )
  }

  if (!user) {
    return <LoginView />
  }

  const isAdmin = Boolean(user && user.role === 'admin')

  return (
    <div className="app-container">
      {/* Top Header */}
      <header className="app-header">
        <div className="brand-group">
          <div className="brand-logo" aria-hidden="true">
            📦
          </div>
          <div>
            <h1 className="brand-title">GoStock</h1>
            <p className="brand-subtitle">Hệ thống quản lý sản phẩm và tồn kho</p>
          </div>
        </div>

        <div className="header-actions">
          <div className="user-account-badge" title={`Tài khoản: ${user.email} (Vai trò: ${user.role})`}>
            <span className="user-email-text">{user.email}</span>
            <span className={`user-role-pill ${isAdmin ? 'role-admin' : 'role-user'}`}>
              {user.role}
            </span>
          </div>

          {isAdmin && (
            <button
              type="button"
              className="btn btn-primary"
              onClick={() => setIsCreateModalOpen(true)}
            >
              <span aria-hidden="true">+</span>
              <span>Thêm sản phẩm</span>
            </button>
          )}

          <button
            type="button"
            className="btn btn-refresh"
            onClick={handleRefresh}
            disabled={loading || refreshing}
            title="Làm mới dữ liệu từ máy chủ"
          >
            <span className={`refresh-icon ${refreshing ? 'rotating' : ''}`} aria-hidden="true">
              ↻
            </span>
            <span>{refreshing ? 'Đang làm mới...' : 'Làm mới'}</span>
          </button>
          <button
            type="button"
            className="btn btn-logout"
            onClick={handleLogout}
            disabled={loggingOut}
            title="Đăng xuất khỏi hệ thống"
          >
            {loggingOut ? (
              <>
                <span className="spinner" aria-hidden="true"></span>
                <span>Đang đăng xuất...</span>
              </>
            ) : (
              <span>Đăng xuất</span>
            )}
          </button>
        </div>
      </header>

      {/* Main Content Area */}
      <main className="main-content">
        {/* Success Alert Banner */}
        {successMessage && (
          <div className="alert alert-success" role="status">
            <div className="alert-content">
              <span className="alert-icon" aria-hidden="true">✓</span>
              <span>{successMessage}</span>
            </div>
            <button
              type="button"
              className="alert-dismiss-btn"
              onClick={() => setSuccessMessage(null)}
              title="Đóng thông báo"
            >
              ✕
            </button>
          </div>
        )}

        {/* Toolbar: Search & Active Filter */}
        <section className="dashboard-toolbar">
          <SearchBar
            value={searchQuery}
            onChange={handleSearchChange}
            disabled={loading && !refreshing}
          />
          <div className="filter-summary">
            {searchQuery && (
              <span className="search-tag">
                Đang tìm: <strong>"{searchQuery}"</strong>
                <button
                  type="button"
                  className="search-tag-remove"
                  onClick={() => handleSearchChange('')}
                  title="Xóa bộ lọc"
                >
                  ✕
                </button>
              </span>
            )}
          </div>
        </section>

        {/* Error Banner with Retry */}
        {error && (
          <div className="alert alert-danger" role="alert">
            <div className="alert-content">
              <strong>Lỗi tải dữ liệu:</strong> {error}
            </div>
            <button
              type="button"
              className="btn btn-retry"
              onClick={() => fetchProducts(false)}
            >
              Thử lại
            </button>
          </div>
        )}

        {/* Product Table & Pagination */}
        <ProductTable
          products={products}
          loading={loading && !refreshing}
          sortBy={sortBy}
          order={order}
          onSort={handleSort}
          page={page}
          limit={limit}
          onPageChange={setPage}
          onLimitChange={handleLimitChange}
          onEdit={(product) => setEditingProductId(product.id)}
          onDelete={(product) => setDeletingProduct(product)}
          onStockIn={handleStockInClick}
          onStockOut={handleStockOutClick}
          isAdmin={isAdmin}
        />
      </main>

      {/* Admin Modals (Create, Edit, Delete) */}
      {isAdmin && (
        <>
          <CreateProductModal
            isOpen={isCreateModalOpen}
            onClose={() => setIsCreateModalOpen(false)}
            onSuccess={handleProductCreated}
          />

          <EditProductModal
            isOpen={editingProductId !== null}
            productId={editingProductId}
            onClose={() => setEditingProductId(null)}
            onSuccess={handleProductUpdated}
          />

          <DeleteConfirmModal
            isOpen={deletingProduct !== null}
            product={deletingProduct}
            onClose={() => setDeletingProduct(null)}
            onSuccess={handleProductDeleted}
          />
        </>
      )}

      {/* Modal Stock In / Stock Out (Dành cho cả User và Admin) */}
      <StockModal
        isOpen={stockModalState.isOpen}
        product={stockModalState.product}
        type={stockModalState.type}
        onClose={() => setStockModalState((prev) => ({ ...prev, isOpen: false }))}
        onSuccess={handleStockSuccess}
      />
    </div>
  )
}
