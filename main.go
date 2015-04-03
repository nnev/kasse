package main

import (
	"fmt"
	"log"
	"time"

	"github.com/fuzxxl/nfc/2.0/nfc"
)

func modulationString(m int) string {
	switch m {
	case nfc.ISO14443a:
		return "ISO14443a"
	case nfc.Jewel:
		return "Jewel"
	case nfc.ISO14443b:
		return "ISO14443b"
	case nfc.ISO14443bi:
		return "ISO14443bi"
	case nfc.ISO14443b2sr:
		return "ISO14443b2sr"
	case nfc.ISO14443b2ct:
		return "ISO14443b2ct"
	case nfc.Felica:
		return "Felica"
	case nfc.DEP:
		return "DEP"
	default:
		return fmt.Sprintf("<unknown: %d>", m)
	}
}

func bitrateString(n int) string {
	switch n {
	case nfc.Nbr106:
		return "Nbr106"
	case nfc.Nbr212:
		return "Nbr212"
	case nfc.Nbr424:
		return "Nbr424"
	case nfc.Nbr847:
		return "Nbr847"
	default:
		return fmt.Sprintf("<unknown: %d>", n)
	}
}

func supportsModulation(ms []int, m int) bool {
	for _, n := range ms {
		if n == m {
			return true
		}
	}
	return false
}

func main() {
	d, err := nfc.Open("")
	if err != nil {
		log.Println(err)
		return
	}
	defer d.Close()

	log.Println(d)

	ms, err := d.SupportedModulations(nfc.InitiatorMode)
	if err != nil {
		log.Println(err)
		return
	}
	for _, n := range ms {
		log.Println("Supported modulation:", modulationString(n))
	}
	if !supportsModulation(ms, nfc.ISO14443a) {
		log.Println("ISO 14443-A not supported")
		return
	}

	br, err := d.SupportedBaudRates(nfc.ISO14443a)
	if err != nil {
		log.Println(err)
		return
	}
	for _, b := range br {
		log.Println("Supported bitrate:", bitrateString(b))
	}

	m := nfc.Modulation{nfc.ISO14443a, br[len(br)-1]}
	log.Printf("Choosing modulation {%s, %s}\n", modulationString(m.Type), bitrateString(m.BaudRate))

	log.Println("Initiate reading")
	if err = d.InitiatorInit(); err != nil {
		log.Println(err)
		return
	}

	for {
		targets, err := d.InitiatorListPassiveTargets(m)
		if err != nil {
			log.Println(err)
			return
		}
		for _, t := range targets {
			log.Println(t)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
