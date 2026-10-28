import { useState, useEffect, useRef } from 'react'

export function SearchBar({ value = '', onChange, disabled = false }) {
  const [searchTerm, setSearchTerm] = useState(value)
  const isFirstRender = useRef(true)

  // Keep internal state in sync with external value prop
  useEffect(() => {
    setSearchTerm(value)
  }, [value])

  // Debounced search (400ms)
  useEffect(() => {
    if (isFirstRender.current) {
      isFirstRender.current = false
      return
    }

    const timer = setTimeout(() => {
      if (searchTerm.trim() !== value.trim()) {
        onChange(searchTerm.trim())
      }
    }, 400)

    return () => clearTimeout(timer)
  }, [searchTerm, onChange, value])

  const handleSubmit = (e) => {
    e.preventDefault()
    onChange(searchTerm.trim())
  }

  const handleClear = () => {
    setSearchTerm('')
    onChange('')
  }

  return (
    <form className="search-bar-form" onSubmit={handleSubmit} role="search">
      <div className="search-input-wrapper">
        <span className="search-icon" aria-hidden="true">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="11" cy="11" r="8"></circle>
            <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
          </svg>
        </span>
        <input
          type="text"
          className="search-input"
          placeholder="Tìm theo tên hoặc mã SKU..."
          value={searchTerm}
          onChange={(e) => setSearchTerm(e.target.value)}
          disabled={disabled}
        />
        {searchTerm && (
          <button
            type="button"
            className="search-clear-btn"
            onClick={handleClear}
            title="Xóa tìm kiếm"
            disabled={disabled}
          >
            ✕
          </button>
        )}
      </div>
      <button type="submit" className="btn btn-secondary" disabled={disabled}>
        Tìm
      </button>
    </form>
  )
}
