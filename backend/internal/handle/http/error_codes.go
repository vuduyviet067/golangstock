package http

// Error codes of errors with a specific meaning. They are set by MapError (for
// use case errors) and by the handlers that report the same error themselves,
// so one message always comes with one code. Errors without a specific code
// use the general codes of gostock/pkg/response. The README lists every code.
const (
	// 400 Bad Request.
	codeInvalidProductID = "INVALID_PRODUCT_ID"
	codeInvalidUserID    = "INVALID_USER_ID"
	codeInvalidUserRole  = "INVALID_USER_ROLE"
	codeEmptyUpdate      = "EMPTY_UPDATE"
	codeInvalidUser      = "INVALID_USER"
	codeInvalidPage      = "INVALID_PAGE"
	codeInvalidLimit     = "INVALID_LIMIT"
	codeInvalidProduct   = "INVALID_PRODUCT"
	codeInvalidQuantity  = "INVALID_QUANTITY"
	codeInvalidEmail     = "INVALID_EMAIL"
	codeInvalidPassword  = "INVALID_PASSWORD"
	// POST /api/v1/auth/change-password. Not 401: the frontend refreshes the
	// session on 401, which a wrong current password must not trigger.
	codeInvalidCurrentPassword = "INVALID_CURRENT_PASSWORD"
	codePasswordUnchanged      = "PASSWORD_UNCHANGED"

	// 401 Unauthorized.
	codeInvalidCredentials = "INVALID_CREDENTIALS"
	codeInvalidSession     = "INVALID_SESSION"
	// One code for every refresh token failure (unknown, revoked, expired,
	// reused, malformed), like the shared message: the client learns no more.
	codeInvalidRefreshToken = "INVALID_REFRESH_TOKEN"

	// 403 Forbidden.
	codeRegistrationDisabled = "REGISTRATION_DISABLED"

	// 404 Not Found.
	codeProductNotFound = "PRODUCT_NOT_FOUND"
	codeUserNotFound    = "USER_NOT_FOUND"

	// 409 Conflict.
	codeConcurrentRefresh = "CONCURRENT_REFRESH"
	codeSKUExists         = "SKU_EXISTS"
	codeEmailExists       = "EMAIL_EXISTS"
	codeInsufficientStock = "INSUFFICIENT_STOCK"
)
