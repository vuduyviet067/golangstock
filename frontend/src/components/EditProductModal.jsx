import { useState, useEffect, useRef } from 'react'
import { getProduct, updateProduct } from '../services/api'

// Largest minimum_stock the API accepts (PostgreSQL INTEGER).
const MAX_MINIMUM_STOCK = 2147483647

const EMPTY_FORM = {
  sku: '',
  name: '',
  price: '',
  minimumStock: '',
}

export function EditProductModal({ isOpen, productId, onClose, onSuccess }) {
  const [formData, setFormData] = useState(EMPTY_FORM)
  // Current stock is shown read-only. It changes only through stock-in/stock-out,
  // and the backend rejects a PUT body that contains quantity.
  const [currentQuantity, setCurrentQuantity] = useState(null)
  // minimum_stock as loaded, so the update sends it only when it changed.
  const [loadedMinimumStock, setLoadedMinimumStock] = useState(null)
  // ID of the product whose data is in the form. The form is shown and can be
  // saved only when it equals productId, so data of a previous product (or of
  // a failed load) is never shown or submitted for the current one.
  const [loadedProductId, setLoadedProductId] = useState(null)
  // Load failure as { productId, message }, tied to the product it concerns.
  const [loadError, setLoadError] = useState(null)
  const [formErrors, setFormErrors] = useState({})
  const [loadingData, setLoadingData] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [apiError, setApiError] = useState(null)

  const nameInputRef = useRef(null)

  // Fetch fresh product details from backend when modal opens
  useEffect(() => {
    if (!isOpen || !productId) return

    let isMounted = true
    let focusTimer = null

    setFormData(EMPTY_FORM)
    setCurrentQuantity(null)
    setLoadedMinimumStock(null)
    setLoadedProductId(null)
    setLoadError(null)
    setLoadingData(true)
    setApiError(null)
    setFormErrors({})

    getProduct(productId)
      .then((prod) => {
        if (!isMounted) return
        const minimumStock = prod.minimum_stock ?? 0
        setFormData({
          sku: prod.sku ?? '',
          name: prod.name ?? '',
          price: prod.price !== undefined && prod.price !== null ? String(prod.price) : '',
          minimumStock: String(minimumStock),
        })
        setCurrentQuantity(prod.quantity ?? null)
        setLoadedMinimumStock(minimumStock)
        setLoadedProductId(productId)
        focusTimer = setTimeout(() => {
          nameInputRef.current?.focus()
        }, 50)
      })
      .catch((err) => {
        if (!isMounted) return
        setLoadError({
          productId,
          message: err?.message || 'Không thể tải thông tin sản phẩm từ máy chủ.',
        })
      })
      .finally(() => {
        if (isMounted) {
          setLoadingData(false)
        }
      })

    return () => {
      // Ignore the response of this request and cancel a pending focus, then
      // forget the loaded product so reopening never shows it before the
      // fresh data arrives.
      isMounted = false
      clearTimeout(focusTimer)
      setLoadedProductId(null)
      setLoadError(null)
    }
  }, [isOpen, productId])

  // ESC key to close modal (only when not submitting)
  useEffect(() => {
    if (!isOpen) return

    const handleKeyDown = (e) => {
      if (e.key === 'Escape' && !submitting) {
        onClose()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [isOpen, submitting, onClose])

  if (!isOpen) return null

  // The form holds this product's data only after its own request succeeded.
  // Until then (including the first render after switching products, before
  // the effect runs) the modal shows the loading state.
  const isLoaded = productId != null && loadedProductId === productId
  const loadFailed = !isLoaded && loadError !== null && loadError.productId === productId
  const showLoading = loadingData || (!isLoaded && !loadFailed)

  const handleChange = (field, value) => {
    setFormData((prev) => ({ ...prev, [field]: value }))
    if (formErrors[field]) {
      setFormErrors((prev) => ({ ...prev, [field]: undefined }))
    }
    if (apiError) {
      setApiError(null)
    }
  }

  // Validate according to backend rules
  const validate = () => {
    const errors = {}
    const trimmedSku = formData.sku.trim()
    const trimmedName = formData.name.trim()

    // SKU: required, max 50 chars
    if (!trimmedSku) {
      errors.sku = 'Vui lòng nhập mã SKU'
    } else if (trimmedSku.length > 50) {
      errors.sku = 'Mã SKU không được vượt quá 50 ký tự'
    }

    // Name: required, max 200 chars
    if (!trimmedName) {
      errors.name = 'Vui lòng nhập tên sản phẩm'
    } else if (trimmedName.length > 200) {
      errors.name = 'Tên sản phẩm không được vượt quá 200 ký tự'
    }

    // Price: required, >= 0
    if (formData.price === '' || formData.price === null || formData.price === undefined) {
      errors.price = 'Vui lòng nhập đơn giá'
    } else {
      const numPrice = Number(formData.price)
      if (isNaN(numPrice)) {
        errors.price = 'Đơn giá phải là số hợp lệ'
      } else if (numPrice < 0) {
        errors.price = 'Đơn giá không được nhỏ hơn 0'
      }
    }

    // Minimum stock: required, integer, 0..MAX_MINIMUM_STOCK
    if (String(formData.minimumStock).trim() === '') {
      errors.minimumStock = 'Vui lòng nhập tồn kho tối thiểu (nhập 0 nếu không cần cảnh báo)'
    } else {
      const numMin = Number(formData.minimumStock)
      if (isNaN(numMin) || !Number.isInteger(numMin)) {
        errors.minimumStock = 'Tồn kho tối thiểu phải là số nguyên'
      } else if (numMin < 0) {
        errors.minimumStock = 'Tồn kho tối thiểu không được nhỏ hơn 0'
      } else if (numMin > MAX_MINIMUM_STOCK) {
        errors.minimumStock = 'Tồn kho tối thiểu quá lớn'
      }
    }

    setFormErrors(errors)
    return Object.keys(errors).length === 0
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!isLoaded || submitting) {
      return
    }
    setApiError(null)

    if (!validate()) {
      return
    }

    setSubmitting(true)
    try {
      // Send only editable fields. quantity is deliberately excluded: the API
      // rejects it on PUT so a stale form value cannot overwrite current stock.
      const payload = {
        sku: formData.sku.trim(),
        name: formData.name.trim(),
        price: Number(formData.price),
      }
      // minimum_stock is sent only when changed; when omitted the API keeps
      // the stored value, so a concurrent change is not overwritten.
      const minimumStock = Number(formData.minimumStock)
      if (minimumStock !== loadedMinimumStock) {
        payload.minimum_stock = minimumStock
      }

      const updated = await updateProduct(loadedProductId, payload)
      onSuccess(updated)
      onClose()
    } catch (err) {
      setApiError(err?.message || 'Không thể cập nhật sản phẩm. Vui lòng kiểm tra lại thông tin.')
    } finally {
      setSubmitting(false)
    }
  }

  const handleBackdropClick = (e) => {
    if (e.target === e.currentTarget && !submitting) {
      onClose()
    }
  }

  return (
    <div
      className="modal-backdrop"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="edit-product-title"
    >
      <div className="modal-container">
        {/* Modal Header */}
        <div className="modal-header">
          <div>
            <h2 id="edit-product-title" className="modal-title">
              Chỉnh sửa sản phẩm #{productId}
            </h2>
            <p className="modal-subtitle">
              Cập nhật thông tin chi tiết của sản phẩm trong kho hàng
            </p>
          </div>
          <button
            type="button"
            className="modal-close-btn"
            onClick={onClose}
            disabled={submitting}
            title="Đóng (Esc)"
            aria-label="Đóng modal"
          >
            ✕
          </button>
        </div>

        {/* Modal Body / Form */}
        <form onSubmit={handleSubmit} className="modal-form" noValidate>
          <div className="modal-body">
            {/* Loading indicator while fetching product data */}
            {showLoading ? (
              <div className="modal-loading-state">
                <span className="spinner" aria-hidden="true"></span>
                <span>Đang tải thông tin sản phẩm...</span>
              </div>
            ) : loadFailed ? (
              <div className="alert alert-danger modal-alert" role="alert">
                <div className="alert-content">
                  <strong>Lỗi:</strong> {loadError.message}
                </div>
              </div>
            ) : (
              <>
                {/* API Error Notification */}
                {apiError && (
                  <div className="alert alert-danger modal-alert" role="alert">
                    <div className="alert-content">
                      <strong>Lỗi:</strong> {apiError}
                    </div>
                  </div>
                )}

                {/* Read-only ID field */}
                <div className="form-group">
                  <label htmlFor="edit-id" className="form-label">
                    Mã hệ thống (ID)
                  </label>
                  <input
                    id="edit-id"
                    type="text"
                    className="form-input form-input-readonly"
                    value={`#${productId}`}
                    disabled
                    readOnly
                  />
                  <span className="form-hint">Mã ID do hệ thống tự sinh và quản lý cố định, không thể sửa đổi.</span>
                </div>

                {/* SKU Field */}
                <div className="form-group">
                  <label htmlFor="edit-sku" className="form-label">
                    Mã SKU <span className="form-required">*</span>
                  </label>
                  <input
                    id="edit-sku"
                    type="text"
                    className={`form-input ${formErrors.sku ? 'has-error' : ''}`}
                    placeholder="VD: KB-001, MS-002..."
                    maxLength={50}
                    value={formData.sku}
                    onChange={(e) => handleChange('sku', e.target.value)}
                    disabled={submitting}
                  />
                  {formErrors.sku && <span className="field-error">{formErrors.sku}</span>}
                  <span className="form-hint">Mã định danh duy nhất của sản phẩm (tối đa 50 ký tự).</span>
                </div>

                {/* Name Field */}
                <div className="form-group">
                  <label htmlFor="edit-name" className="form-label">
                    Tên sản phẩm <span className="form-required">*</span>
                  </label>
                  <input
                    ref={nameInputRef}
                    id="edit-name"
                    type="text"
                    className={`form-input ${formErrors.name ? 'has-error' : ''}`}
                    placeholder="VD: Bàn phím cơ không dây..."
                    maxLength={200}
                    value={formData.name}
                    onChange={(e) => handleChange('name', e.target.value)}
                    disabled={submitting}
                  />
                  {formErrors.name && <span className="field-error">{formErrors.name}</span>}
                  <span className="form-hint">Tên hiển thị chi tiết (tối đa 200 ký tự).</span>
                </div>

                {/* Price & Quantity in two columns */}
                <div className="form-row">
                  <div className="form-group">
                    <label htmlFor="edit-price" className="form-label">
                      Đơn giá (₫) <span className="form-required">*</span>
                    </label>
                    <input
                      id="edit-price"
                      type="number"
                      min="0"
                      step="any"
                      className={`form-input ${formErrors.price ? 'has-error' : ''}`}
                      placeholder="0"
                      value={formData.price}
                      onChange={(e) => handleChange('price', e.target.value)}
                      disabled={submitting}
                    />
                    {formErrors.price && <span className="field-error">{formErrors.price}</span>}
                  </div>

                  <div className="form-group">
                    <label htmlFor="edit-quantity" className="form-label">
                      Số lượng tồn kho
                    </label>
                    <input
                      id="edit-quantity"
                      type="text"
                      className="form-input"
                      value={currentQuantity ?? '—'}
                      readOnly
                      disabled
                    />
                    <span className="form-hint">Chỉ thay đổi qua Nhập kho / Xuất kho.</span>
                  </div>
                </div>

                {/* Minimum Stock Field */}
                <div className="form-group">
                  <label htmlFor="edit-minimum-stock" className="form-label">
                    Tồn kho tối thiểu <span className="form-required">*</span>
                  </label>
                  <input
                    id="edit-minimum-stock"
                    type="number"
                    min="0"
                    step="1"
                    className={`form-input ${formErrors.minimumStock ? 'has-error' : ''}`}
                    placeholder="0"
                    value={formData.minimumStock}
                    onChange={(e) => handleChange('minimumStock', e.target.value)}
                    disabled={submitting}
                  />
                  {formErrors.minimumStock && <span className="field-error">{formErrors.minimumStock}</span>}
                  <span className="form-hint">
                    Sản phẩm được coi là sắp hết khi tồn kho lớn hơn 0 và không vượt quá ngưỡng này. Nhập 0 nếu không cần cảnh báo.
                  </span>
                </div>
              </>
            )}
          </div>

          {/* Modal Footer Actions */}
          <div className="modal-footer">
            <button
              type="button"
              className="btn btn-cancel"
              onClick={onClose}
              disabled={submitting}
            >
              Hủy
            </button>
            <button
              type="submit"
              className="btn btn-primary"
              disabled={submitting || !isLoaded}
            >
              {submitting ? 'Đang lưu...' : 'Lưu thay đổi'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
