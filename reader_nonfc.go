// +build nonfc

package main

// ConnectAndPollNFCReader is a stub to enable a build without libnfc. It
// blocks indefinitely.
func ConnectAndPollNFCReader(conn string, ch chan NFCEvent) error {
	block := make(chan bool)
	<-block
	return nil
}
