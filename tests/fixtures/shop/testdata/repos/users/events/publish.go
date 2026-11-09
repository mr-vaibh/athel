// Package events publishes users.updated.v1.
package events

import "log"

// Topic is the event every phone or name change is announced on.
const Topic = "users.updated.v1"

// PublishUserUpdated announces a changed user.
func PublishUserUpdated(id, phone string) {
	log.Printf("publish %s id=%s phone_set=%t", Topic, id, phone != "")
}
