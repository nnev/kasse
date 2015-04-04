package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	r, err := NewNFCReader("")
	if err != nil {
		log.Fatal(err)
	}

	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	uids := make(chan []byte)
	go func() {
		for {
			uid, err := r.GetNextUID()
			if err != nil {
				log.Println(err)
				close(sigs)
				return
			}
			uids <- uid
		}
	}()

	for {
		select {
		case uid := <-uids:
			log.Printf("Got UID %x", uid)
		case sig := <-sigs:
			if sig != nil {
				log.Printf("Got signal %s, exiting\n", sig)
			}
			if err := r.Close(); err != nil {
				log.Println("Error closing reader:", err)
			}
			return
		}
	}
}
