package clients

import (
	"net/http"
	"strings"
)

// Payments reaches payments only through the gateway (route /payments/*).
type Payments struct{ Base string }

// Charge charges an order.
func (p Payments) Charge(orderID string) (*http.Response, error) {
	return http.Post(p.Base+"/payments/charge", "application/json", strings.NewReader(`{"order":"`+orderID+`"}`)) //nolint:gosec,noctx // fixture code, never run
}
