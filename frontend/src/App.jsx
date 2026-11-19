import { useState, useEffect, useCallback, useRef } from 'react'
import { getProducts } from './services/api'
import { useAuth } from './contexts/AuthContext'
import { MAIN_TAB_LABELS, resolveActiveTab, tabOwner, visibleTabs } from './utils/dashboardTabs'
import { LoginView } from './components/LoginView'
import { mainPanelId, mainTabId } from './components/MainTabs'
import { AppShell } from './components/AppShell'
import { SidebarNav } from './components/SidebarNav'
import { AccountPanel } from './components/AccountPanel'
import { PlusIcon, RefreshIcon } from './components/icons'
import { DashboardPanel } from './components/DashboardPanel'
import { SearchBar } from './components/SearchBar'
import { ProductTable } from './components/ProductTable'
import { CreateProductModal } from './components/CreateProductModal'
import { EditProductModal } from './components/EditProductModal'
import { DeleteConfirmModal } from './components/DeleteConfirmModal'
import { StockModal } from './components/StockModal'
import './App.css'

// Everything that belongs to a signed-in account lives in AuthenticatedApp,
// keyed by the account (id and role). It is unmounted while signed out and
// while the session is re-checked (checkAuth shows the loading screen), and
// remounted when the account or its role changes. So the products, search,
// paging, banners, open modals, tab selection and any request in flight of
// one account never reach the next one: unmounting drops the state and the
// effect cleanups abort the requests.
export default function App() {
  const { user, loading: authLoading, logout, checkAuth } = useAuth()

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

  return <AuthenticatedApp key={tabOwner(user)} user={user} logout={logout} checkAuth={checkAuth} />
}

function AuthenticatedApp({ user, logout, checkAuth }) {
  const [loggingOut, setLoggingOut] = useState(false)

  // The active tab is derived on every render: a selection only counts for
  // the account (id and role) that made it. A new login always starts on the
  // default tab (Dashboard for an admin, Sản phẩm for a user) because this
  // component is mounted afresh for it.
  const [tabSelection, setTabSelection] = useState(null)
  const tabs = visibleTabs(user)
  const activeTab = resolveActiveTab(user, tabSelection)
  const productsTabActive = activeTab === 'products'
  const handleTabChange = (tab) => {
    setTabSelection({ owner: tabOwner(user), tab })
  }

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

  // At most one product request runs at a time: a new one aborts the previous
  // one, and leaving the Sản phẩm tab (or signing out) aborts it too. An
  // aborted request never updates the state.
  const productRequest = useRef(null)

  const cancelProductRequest = useCallback(() => {
    productRequest.current?.abort()
    productRequest.current = null
  }, [])

  // False once this account's view is unmounted (sign-out, session check,
  // account or role change): a modal of the previous account that finishes
  // afterwards must not start a product request for it. Declared before the
  // products effect so it is set first.
  const mounted = useRef(false)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      cancelProductRequest()
    }
  }, [cancelProductRequest])

  const fetchProducts = useCallback(async (isManualRefresh = false) => {
    if (!mounted.current) return
    productRequest.current?.abort()
    const controller = new AbortController()
    productRequest.current = controller

    setRefreshing(isManualRefresh)
    setLoading(!isManualRefresh)
    setError(null)

    try {
      const data = await getProducts(
        {
          q: searchQuery,
          page,
          limit,
          sortBy,
          order,
        },
        { signal: controller.signal }
      )
      if (controller.signal.aborted) return
      setProducts(Array.isArray(data) ? data : [])
    } catch (err) {
      if (controller.signal.aborted) return
      setError(err?.message || 'Không thể tải danh sách sản phẩm từ máy chủ.')
      setProducts([])
    } finally {
      if (productRequest.current === controller) {
        productRequest.current = null
        setLoading(false)
        setRefreshing(false)
      }
    }
  }, [searchQuery, page, limit, sortBy, order])

  // Load the products whenever the Sản phẩm tab is shown, so they are fresh
  // after a visit to the Dashboard, and again when the query changes.
  useEffect(() => {
    if (!productsTabActive) return
    fetchProducts()
    return cancelProductRequest
  }, [fetchProducts, productsTabActive, cancelProductRequest])

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

  const isAdmin = Boolean(user && user.role === 'admin')

  return (
    <>
    <AppShell
      navigation={(closeDrawer) => (
        <SidebarNav tabs={tabs} activeTab={activeTab} onChange={handleTabChange} onActivate={closeDrawer} />
      )}
      account={() => <AccountPanel user={user} loggingOut={loggingOut} onLogout={handleLogout} />}
    >
      {/* Dashboard tab: rendered for admins only (activeTab can only be
          'dashboard' for an admin); the backend still answers 403 to a user. */}
      {activeTab === 'dashboard' && (
        <main
          className="main-content"
          role="tabpanel"
          id={mainPanelId('dashboard')}
          aria-labelledby={mainTabId('dashboard')}
        >
          <header className="page-header">
            <div className="page-heading">
              <h1 className="page-title">{MAIN_TAB_LABELS.dashboard}</h1>
              <p className="page-subtitle">Số liệu tồn kho và biến động nhập – xuất kho.</p>
            </div>
          </header>
          <DashboardPanel onReauthenticate={checkAuth} />
        </main>
      )}

      {/* Main Content Area */}
      {productsTabActive && (
        <main
          className="main-content"
          role="tabpanel"
          id={mainPanelId('products')}
          aria-labelledby={mainTabId('products')}
        >
          <header className="page-header">
            <div className="page-heading">
              <h1 className="page-title">{MAIN_TAB_LABELS.products}</h1>
              <p className="page-subtitle">Danh mục sản phẩm, tồn kho và thao tác nhập – xuất kho.</p>
            </div>

            {/* Product actions belong to the Sản phẩm tab */}
            <div className="page-actions">
              {isAdmin && (
                <button
                  type="button"
                  className="btn btn-primary"
                  onClick={() => setIsCreateModalOpen(true)}
                >
                  <PlusIcon size={18} />
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
                <RefreshIcon size={18} className={`refresh-icon ${refreshing ? 'rotating' : ''}`} />
                <span>{refreshing ? 'Đang làm mới...' : 'Làm mới'}</span>
              </button>
            </div>
          </header>

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
      )}
    </AppShell>

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
    </>
  )
}
