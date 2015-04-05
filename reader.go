package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/fuzxxl/nfc/2.0/nfc"
	"golang.org/x/net/context"
)

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

// Reader abstracts the NFC Reader functions we need, for testing. A Reader
// must be safe for concurrent use.
type Reader interface {
	// GetNextUID blocks until a new card is put on the reader and returns the
	// UID of the card.
	GetNextUID() (uid []byte, err error)
	// Close closes the Reader and cleans up any resources held.
	Close() error
}

type nfcReader struct {
	nfc.Device
	m nfc.Modulation
	// uids is used to pass card-uids between goroutines
	uids chan []byte
	// errs passes errors from polling
	errs chan error
	// used for threadsafe cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

func (n nfcReader) Close() error {
	n.cancel()
	return n.Device.Close()
}

func (n nfcReader) GetNextUID() (uid []byte, err error) {
	select {
	case <-n.ctx.Done():
		return nil, errors.New("reader is closed")
	case err := <-n.errs:
		return nil, err
	case uid := <-n.uids:
		return uid, nil
	}
}

// startPolling polls for new cards in the reader, until n.done is closed.
func (n nfcReader) startPolling() {
	for {
		targets, err := n.Device.InitiatorListPassiveTargets(n.m)
		if err != nil {
			// pass error, if not closed and abort polling
			select {
			case <-n.ctx.Done():
			case n.errs <- err:
			}
			return
		}
		// TODO: Should we ever get more than one target? Better not (because of
		// clash-prevention in the reader), but we will just handle it for now.
		for _, t := range targets {
			// TODO: Handle other target types
			tt, ok := t.(*nfc.ISO14443aTarget)
			if !ok {
				select {
				case <-n.ctx.Done():
				case n.errs <- fmt.Errorf("unsupported card type %T", t):
				}
				return
			}
			select {
			case <-n.ctx.Done():
				return
			case n.uids <- tt.UID[:tt.UIDLen]:
			}
		}

		select {
		case <-time.After(PollingInterval):
		case <-n.ctx.Done():
			return
		}
	}
}

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

// NewNFCReader returns a new Reader that uses libnfc to access a physical NFC
// reader. conn is the reader to connect to - if empty, the first available
// reader will be used.
func NewNFCReader(conn string) (r Reader, err error) {
	if DefaultModulation.Type != nfc.ISO14443a {
		return nil, errors.New("only ISO 14443-A readers are supported for now")
	}

	d, err := nfc.Open(conn)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			d.Close()
		}
	}()

	log.Printf("NFC reader information:\n%s\n", d)

	ms, err := d.SupportedModulations(nfc.InitiatorMode)
	if err != nil {
		return nil, err
	}

	for _, m := range ms {
		log.Println("Supported modulation type:", modulationString(m))
	}
	if len(ms) == 0 {
		return nil, errors.New("no modulation types supported")
	}

	var m int
	if contains(ms, DefaultModulation.Type) {
		m = DefaultModulation.Type
	} else {
		m = ms[0]
	}

	bs, err := d.SupportedBaudRates(m)
	if err != nil {
		return nil, err
	}
	if len(bs) == 0 {
		return nil, errors.New("no baudrates supported at used modulation")
	}

	var b int
	if contains(bs, DefaultModulation.BaudRate) {
		b = DefaultModulation.BaudRate
	} else {
		b = bs[0]
	}

	if err = d.InitiatorInit(); err != nil {
		return nil, err
	}

	nr := nfcReader{
		Device: d,
		m:      nfc.Modulation{Type: m, BaudRate: b},
		uids:   make(chan []byte),
		errs:   make(chan error),
	}
	nr.ctx, nr.cancel = context.WithCancel(context.Background())

	go nr.startPolling()
	return nr, nil
}
