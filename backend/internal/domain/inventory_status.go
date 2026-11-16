package domain

// Inventory statuses. With quantity >= 0 and minimum_stock >= 0 every product
// is in exactly one: normal (quantity > minimum_stock), low_stock
// (0 < quantity <= minimum_stock) or out_of_stock (quantity = 0).
const (
	InventoryStatusNormal     = "normal"
	InventoryStatusLowStock   = "low_stock"
	InventoryStatusOutOfStock = "out_of_stock"
)
