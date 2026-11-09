// Package main is the orders service.
package main

import (
	"log"

	"shop/orders/clients"
)

func main() {
	u := clients.Users{Base: "http://user-svc:8080"}
	p := clients.Payments{Base: "http://gateway:8080"}
	log.Println(u.Base, p.Base)
}
