package client

import "time"

type Order struct {
	ID        int64     `json:"id"`
	Customer  string    `json:"customer"`
	Product   string    `json:"product"`
	Quantity  int       `json:"quantity"`
	UnitPrice float64   `json:"unit_price"`
	Total     float64   `json:"total"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
type listResponse struct {
	Data  []Order `json:"data"`
	Count int     `json:"count"`
}
