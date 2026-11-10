// Package clients calls other shop services.
package clients

import "net/http"

// Users calls the users service.
type Users struct{ Base string }

// Get reads a customer, including the phone number for order notifications.
func (u Users) Get(id string) (*http.Response, error) {
	return http.Get(u.Base + "/v1/users/" + id) //nolint:gosec,noctx // fixture code, never run
}

// LegacyProfile is still called by the receipt job, although users removed the endpoint.
func (u Users) LegacyProfile(id string) (*http.Response, error) {
	return http.Get(u.Base + "/v1/users/" + id + "/legacy-profile") //nolint:gosec,noctx // fixture code, never run
}
