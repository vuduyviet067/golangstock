import { useState, useEffect, useRef } from 'react'
import { createProduct } from '../services/api'

export function CreateProductModal({ isOpen, onClose, onSuccess }) {
  const [formData, setFormData] = useState({
    sku: '',
    name: '',
    price: '',
    quantity: '0',
  })
  const [formErrors, setFormErrors] = useState({})
  const [submitting, setSubmitting] = useState(false)
  const [apiError, setApiError] = useState(null)

  const skuInputRef = useRef(null)

  // Focus the first input when opened & reset form on close/open
  useEffect(() => {
    if (isOpen) {
      setFormData({
        sku: '',
        name: '',
        price: '',
        quantity: '0',
      })
      setFormErrors({})
      setApiError(null)

      // Slight timeout to let DOM render
      const timer = setTimeout(() => {
        skuInputRef.current?.focus()
      }, 50)
      return () => clearTimeout(timer)
    }
  }, [isOpen])

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

  const handleChange = (field, value) => {
    setFormData((prev) => ({ ...prev, [field]: value }))
    // Clear field-level error on change
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

    // Quantity: required, integer, >= 0
    if (formData.quantity === '' || formData.quantity === null || formData.quantity === undefined) {
      errors.quantity = 'Vui lòng nhập số lượng tồn kho'
    } else {
      const numQty = Number(formData.quantity)
      if (isNaN(numQty) || !Number.isInteger(numQty)) {
        errors.quantity = 'Số lượng phải là số nguyên'
      } else if (numQty < 0) {
        errors.quantity = 'Số lượng tồn kho không được nhỏ hơn 0'
      }
    }

    setFormErrors(errors)
    return Object.keys(errors).length === 0
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setApiError(null)

    if (!validate()) {
      return
    }

    setSubmitting(true)
    try {
      // Send only expected fields to satisfy decoder.DisallowUnknownFields()
      const payload = {
        sku: formData.sku.trim(),
        name: formData.name.trim(),
        price: Number(formData.price),
        quantity: Number(formData.quantity),
      }

      const created = await createProduct(payload)
      onSuccess(created)
      onClose()
    } catch (err) {
      setApiError(err?.message || 'Không thể tạo sản phẩm. Vui lòng kiểm tra lại thông tin.')
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
      aria-labelledby="create-product-title"
    >
      <div className="modal-container">
        {/* Modal Header */}
        <div className="modal-header">
          <div>
            <h2 id="create-product-title" className="modal-title">
              Thêm sản phẩm mới
            </h2>
            <p className="modal-subtitle">
              Điền thông tin chi tiết để thêm sản phẩm vào hệ thống kho hàng
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
            {/* API Error Notification */}
            {apiError && (
              <div className="alert alert-danger modal-alert" role="alert">
                <div className="alert-content">
                  <strong>Lỗi:</strong> {apiError}
                </div>
              </div>
            )}

            {/* SKU Field */}
            <div className="form-group">
              <label htmlFor="create-sku" className="form-label">
                Mã SKU <span className="form-required">*</span>
              </label>
              <input
                ref={skuInputRef}
                id="create-sku"
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
              <label htmlFor="create-name" className="form-label">
                Tên sản phẩm <span className="form-required">*</span>
              </label>
              <input
                id="create-name"
                type="text"
                className={`form-input ${formErrors.name ? 'has-error' : ''}`}
                placeholder="VD: Bàn phím cơ không dây, Chuột gaming..."
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
                <label htmlFor="create-price" className="form-label">
                  Đơn giá (₫) <span className="form-required">*</span>
                </label>
                <input
                  id="create-price"
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
                <label htmlFor="create-quantity" className="form-label">
                  Số lượng tồn kho <span className="form-required">*</span>
                </label>
                <input
                  id="create-quantity"
                  type="number"
                  min="0"
                  step="1"
                  className={`form-input ${formErrors.quantity ? 'has-error' : ''}`}
                  placeholder="0"
                  value={formData.quantity}
                  onChange={(e) => handleChange('quantity', e.target.value)}
                  disabled={submitting}
                />
                {formErrors.quantity && <span className="field-error">{formErrors.quantity}</span>}
              </div>
            </div>
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
              disabled={submitting}
            >
              {submitting ? 'Đang tạo sản phẩm...' : 'Tạo sản phẩm'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
