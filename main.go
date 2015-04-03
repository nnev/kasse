package main

import "log"

func main() {
	r, err := NewNFCReader("")
	if err != nil {
		log.Fatal(err)
	}
	for {
		uid, err := r.GetNextUID()
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Got UID %x", uid)
	}
}
