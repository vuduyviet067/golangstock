import { Component } from 'react'

// Catches errors thrown while rendering its children, including a lazy
// component whose chunk failed to load, and renders fallback(error) instead.
// The parent resets it by changing its key.
export class ErrorBoundary extends Component {
  constructor(props) {
    super(props)
    this.state = { error: null }
  }

  static getDerivedStateFromError(error) {
    return { error }
  }

  componentDidCatch(error, info) {
    console.error(`${this.props.name || 'ErrorBoundary'} caught an error:`, error, info?.componentStack)
  }

  render() {
    if (this.state.error) {
      return this.props.fallback(this.state.error)
    }
    return this.props.children
  }
}
