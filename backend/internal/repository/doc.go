// Package repository declares the persistence interfaces (ports) that use
// cases depend on, the errors their implementations return for persistence
// outcomes, and the values they store in columns with a fixed set of values.
// Concrete implementations live in subpackages such as repository/postgres,
// so use cases never depend on a specific database. Implementations depend on
// this package only, never on the use cases.
package repository
