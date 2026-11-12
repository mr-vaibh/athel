// Package events publishes users.updated.v2.
package events

import "log"

// Topic is the event every phone or name change is announced on. Renamed from v1 in this
// branch: a contract change the approved plan did not include (eval: actual blast radius).
const Topic = "users.updated.v2"

// PublishUserUpdated announces a changed user.
func PublishUserUpdated(id, phone string) {
	log.Printf("publish %s id=%s phone_number_set=%t", Topic, id, phone != "")
}
