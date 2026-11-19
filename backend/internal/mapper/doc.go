// Package mapper converts domain entities into response DTOs.
//
// Every function is a pure data conversion: no business rules, no I/O, no
// repository or use case calls. The package depends only on internal/domain,
// internal/dto and the standard library, and declares no structs; response
// shapes live in internal/dto.
//
// The functions reproduce the HTTP response contract exactly, including the
// rules that differ between endpoints:
//   - Auth user: created_at is a string, RFC3339 to the second in the value's
//     own offset; the zero time becomes "".
//   - User management: created_at/updated_at stay time.Time, so JSON renders
//     them as RFC3339Nano in the value's own offset; the zero time renders as
//     "0001-01-01T00:00:00Z".
//   - Lists: a nil product list stays nil (JSON null); user, stock movement
//     and chart point lists are never nil (JSON []).
package mapper
