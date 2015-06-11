package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/fuzxxl/nfc/2.0/nfc"
)

// NFCEvent contains an event at the NFC reader. Either UID or Err is nil.
type NFCEvent struct {
	UID []byte
	Err error
}

// DefaultModulation gives defaults for the modulation type and Baudrate.
// Currently, only nfc.ISO14443a is supported for the type. If the default
// BaudRate is not supported by the reader, the fallback is the lowest
// supported value.
var DefaultModulation = nfc.Modulation{
	Type:     nfc.ISO14443a,
	BaudRate: nfc.Nbr106,
}

// PollingInterval gives the interval of polling for new cards.
var PollingInterval = 100 * time.Millisecond

func contains(haystack []int, needle int) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

var modulationStrings = map[int]string{
	nfc.ISO14443a:    "ISO 14443-A",
	nfc.Jewel:        "Jewel",
	nfc.ISO14443b:    "ISO 14443-B",
	nfc.ISO14443bi:   "ISO 14443-B'",
	nfc.ISO14443b2sr: "ISO 14443-2B ST SRx",
	nfc.ISO14443b2ct: "ISO 14443-2B ASK CTx",
	nfc.Felica:       "Felica",
	nfc.DEP:          "DEP",
}

func modulationString(m int) string {
	if s, ok := modulationStrings[m]; ok {
		return s
	}
	return fmt.Sprintf("<unknown: %d>", m)
}

var bitrateStrings = map[int]string{
	nfc.Nbr106: "Nbr106",
	nfc.Nbr212: "Nbr212",
	nfc.Nbr424: "Nbr424",
	nfc.Nbr847: "Nbr847",
}

func bitrateString(n int) string {
	if s, ok := bitrateStrings[n]; ok {
		return s
	}
	return fmt.Sprintf("<unknown: %d>", n)
}

func pollNFC(d nfc.Device, m nfc.Modulation) (uid []byte, err error) {
	targets, err := d.InitiatorListPassiveTargets(m)
	if err != nil {
		return nil, err
	}

	if len(targets) == 0 {
		return nil, nil
	}

	// We assume, that clash-prevention in the reader gives us always
	// exactly one target.
	if len(targets) != 1 {
		log.Printf("Card-clash! Only using first target")
	}

	t := targets[0]
	// TODO: Handle other target types
	tt, ok := t.(*nfc.ISO14443aTarget)
	if !ok {
		return nil, fmt.Errorf("unsupported card type %T", t)
	}
	return tt.UID[:tt.UIDLen], nil
}

// ConnectAndPollNFCReader connects to a physical NFC Reader and pools for new
// cards. conn is the reader to connect to - if empty, the first available
// reader will be used.
func ConnectAndPollNFCReader(conn string, ch chan NFCEvent) error {
	if DefaultModulation.Type != nfc.ISO14443a {
		return errors.New("only ISO 14443-A readers are supported for now")
	}

	d, err := nfc.Open(conn)
	if err != nil {
		return err
	}
	defer d.Close()

	log.Printf("NFC reader information:\n%s\n", d)

	ms, err := d.SupportedModulations(nfc.InitiatorMode)
	if err != nil {
		return err
	}

	for _, m := range ms {
		log.Println("Supported modulation type:", modulationString(m))
	}
	if len(ms) == 0 {
		return errors.New("no modulation types supported")
	}

	var m int
	if contains(ms, DefaultModulation.Type) {
		m = DefaultModulation.Type
	} else {
		m = ms[0]
	}

	bs, err := d.SupportedBaudRates(m)
	if err != nil {
		return err
	}
	if len(bs) == 0 {
		return errors.New("no baudrates supported at used modulation")
	}

	var b int
	if contains(bs, DefaultModulation.BaudRate) {
		b = DefaultModulation.BaudRate
	} else {
		b = bs[0]
	}

	if err = d.InitiatorInit(); err != nil {
		return err
	}

	mod := nfc.Modulation{Type: m, BaudRate: b}

	// start polling
	for {
		uid, err := pollNFC(d, mod)
		if uid == nil && err == nil {
			time.Sleep(PollingInterval)
			continue
		}
		ch <- NFCEvent{uid, err}
	}
}
