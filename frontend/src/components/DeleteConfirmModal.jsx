import { useState, useEffect } from 'react'
import { deleteProduct } from '../services/api'

export function DeleteConfirmModal({ isOpen, product, onClose, onSuccess }) {
  const [submitting, setSubmitting] = useState(false)
  const [apiError, setApiError] = useState(null)

  // Reset error when modal opens or closes
  useEffect(() => {
    if (isOpen) {
      setApiError(null)
      setSubmitting(false)
    }
  }, [isOpen, product])

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

  const handleConfirmDelete = async () => {
    setSubmitting(true)
    setApiError(null)

    try {
      await deleteProduct(product.id)
      onSuccess(product)
      onClose()
    } catch (err) {
      setApiError(err?.message || 'Không thể xóa sản phẩm. Vui lòng thử lại.')
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
      aria-labelledby="delete-dialog-title"
    >
      <div className="modal-container modal-container-sm">
        {/* Header */}
        <div className="modal-header">
          <div>
            <h2 id="delete-dialog-title" className="modal-title modal-title-danger">
              Xác nhận xóa sản phẩm
            </h2>
            <p className="modal-subtitle">
              Hành động này sẽ xóa vĩnh viễn sản phẩm khỏi kho và không thể hoàn tác
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
        <div className="modal-body">
          {apiError && (
            <div className="alert alert-danger modal-alert" role="alert">
              <div className="alert-content">
                <strong>Lỗi:</strong> {apiError}
              </div>
            </div>
          )}

          <div className="confirm-delete-box">
            <span className="warning-icon" aria-hidden="true">⚠️</span>
            <span>
              Bạn có chắc chắn muốn xóa sản phẩm dưới đây không?
            </span>
          </div>

          <div className="product-detail-preview">
            <div className="preview-row">
              <span className="preview-label">Mã hệ thống (ID):</span>
              <span className="preview-value">#{product.id}</span>
            </div>
            <div className="preview-row">
              <span className="preview-label">Mã SKU:</span>
              <span className="preview-value sku-code">{product.sku}</span>
            </div>
            <div className="preview-row">
              <span className="preview-label">Tên sản phẩm:</span>
              <span className="preview-value">{product.name}</span>
            </div>
            <div className="preview-row">
              <span className="preview-label">Số lượng tồn kho:</span>
              <span className="preview-value">{product.quantity}</span>
            </div>
          </div>
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
            type="button"
            className="btn btn-danger"
            onClick={handleConfirmDelete}
            disabled={submitting}
          >
            {submitting ? 'Đang xóa...' : 'Xác nhận xóa'}
          </button>
        </div>
      </div>
    </div>
  )
}
