package main

import (
	"log"

	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
