import { useState, useEffect, useRef } from 'react'
import { stockIn, stockOut } from '../services/api'

export function StockModal({ isOpen, product, type = 'in', onClose, onSuccess }) {
  const [quantity, setQuantity] = useState('')
  const [fieldError, setFieldError] = useState(null)
  const [submitting, setSubmitting] = useState(false)
  const [apiError, setApiError] = useState(null)

  const inputRef = useRef(null)
  const isStockIn = type === 'in'

  useEffect(() => {
    if (isOpen) {
      setQuantity('')
      setFieldError(null)
      setApiError(null)
      setSubmitting(false)

      const timer = setTimeout(() => {
        inputRef.current?.focus()
      }, 50)
      return () => clearTimeout(timer)
    }
  }, [isOpen, product, type])

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

  if (!isOpen || !product) return null

  const handleQuantityChange = (val) => {
    setQuantity(val)
    if (fieldError) setFieldError(null)
    if (apiError) setApiError(null)
  }

  const validate = () => {
    if (!quantity || quantity.trim() === '') {
      setFieldError('Vui lòng nhập số lượng')
      return false
    }

    const num = Number(quantity)
    if (isNaN(num)) {
      setFieldError('Số lượng phải là một số hợp lệ')
      return false
    }

    if (!Number.isInteger(num)) {
      setFieldError('Số lượng phải là số nguyên (không chấp nhận số thập phân)')
      return false
    }

    if (num <= 0) {
      setFieldError('Số lượng phải lớn hơn 0')
      return false
    }

    setFieldError(null)
    return true
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setApiError(null)

    if (!validate()) {
      return
    }

    const numQty = Number(quantity)
    setSubmitting(true)

    try {
      let updated
      if (isStockIn) {
        updated = await stockIn(product.id, numQty)
      } else {
        updated = await stockOut(product.id, numQty)
      }

      onSuccess(updated, isStockIn ? 'in' : 'out', numQty)
      onClose()
    } catch (err) {
      let errorMsg = err?.message || 'Giao dịch thất bại. Vui lòng thử lại.'
      if (err?.status === 409 || errorMsg.toLowerCase().includes('insufficient stock')) {
        errorMsg = 'Insufficient stock (Không đủ hàng tồn kho để thực hiện xuất kho)'
      }
      setApiError(errorMsg)
    } finally {
      setSubmitting(false)
    }
  }

  const handleBackdropClick = (e) => {
    if (e.target === e.currentTarget && !submitting) {
      onClose()
    }
  }

  // Preview calculation
  const parsedQty = Number(quantity)
  const isValidInteger = !isNaN(parsedQty) && Number.isInteger(parsedQty) && parsedQty > 0
  const projectedStock = isValidInteger
    ? (isStockIn ? product.quantity + parsedQty : product.quantity - parsedQty)
    : null

  return (
    <div
      className="modal-backdrop"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="stock-modal-title"
    >
      <div className="modal-container modal-container-sm">
        {/* Header */}
        <div className="modal-header">
          <div>
            <h2 id="stock-modal-title" className="modal-title">
              {isStockIn ? 'Nhập kho sản phẩm' : 'Xuất kho sản phẩm'}
            </h2>
            <p className="modal-subtitle">
              {isStockIn
                ? 'Tăng số lượng hàng tồn kho cho sản phẩm'
                : 'Giảm số lượng hàng tồn kho của sản phẩm'}
            </p>
          </div>
          <button
            type="button"
            className="modal-close-btn"
            onClick={onClose}
            disabled={submitting}
            title="Đóng (Esc)"
            aria-label="Đóng hộp thoại"
          >
            ✕
          </button>
        </div>

        {/* Body */}
        <form onSubmit={handleSubmit} className="modal-form" noValidate>
          <div className="modal-body">
            {apiError && (
              <div className="alert alert-danger modal-alert" role="alert">
                <div className="alert-content">
                  <strong>Lỗi:</strong> {apiError}
                </div>
              </div>
            )}

            {/* Product summary card */}
            <div className="product-detail-preview">
              <div className="preview-row">
                <span className="preview-label">Mã SKU:</span>
                <span className="preview-value sku-code">{product.sku}</span>
              </div>
              <div className="preview-row">
                <span className="preview-label">Tên sản phẩm:</span>
                <span className="preview-value font-medium">{product.name}</span>
              </div>
              <div className="preview-row">
                <span className="preview-label">Tồn kho hiện tại:</span>
                <span className="preview-value">{product.quantity}</span>
              </div>
            </div>

            {/* Quantity Input */}
            <div className="form-group">
              <label htmlFor="stock-quantity-input" className="form-label">
                {isStockIn ? 'Số lượng nhập thêm' : 'Số lượng xuất bớt'} <span className="form-required">*</span>
              </label>
              <input
                ref={inputRef}
                id="stock-quantity-input"
                type="number"
                min="1"
                step="1"
                className={`form-input ${fieldError ? 'has-error' : ''}`}
                placeholder="Nhập số nguyên dương (VD: 10)"
                value={quantity}
                onChange={(e) => handleQuantityChange(e.target.value)}
                disabled={submitting}
              />
              {fieldError && <span className="field-error">{fieldError}</span>}
              <span className="form-hint">Chỉ chấp nhận số nguyên lớn hơn 0.</span>
            </div>

            {/* Projected Stock Preview */}
            {projectedStock !== null && (
              <div className={`stock-preview-box ${!isStockIn && projectedStock < 0 ? 'stock-preview-danger' : ''}`}>
                <div className="stock-preview-row">
                  <span>Tồn kho sau khi {isStockIn ? 'nhập' : 'xuất'}:</span>
                  <strong>{projectedStock}</strong>
                </div>
                {!isStockIn && projectedStock < 0 && (
                  <p className="stock-preview-warning">
                    ⚠️ Số lượng xuất vượt quá tồn kho hiện tại ({product.quantity}). Yêu cầu sẽ bị máy chủ từ chối với mã 409 (Insufficient stock).
                  </p>
                )}
              </div>
            )}
          </div>

          {/* Footer */}
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
              className={`btn ${isStockIn ? 'btn-primary' : 'btn-stock-submit'}`}
              disabled={submitting}
            >
              {submitting
                ? 'Đang xử lý...'
                : isStockIn
                ? 'Xác nhận nhập kho'
                : 'Xác nhận xuất kho'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
