// Package repository declares the persistence interfaces (ports) that use
// cases depend on. It contains interfaces only; concrete implementations live
// in subpackages such as repository/postgres, so use cases never depend on a
// specific database.
package repository
