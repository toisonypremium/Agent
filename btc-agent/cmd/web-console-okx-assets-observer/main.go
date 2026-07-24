// web-console-okx-assets-observer obtains a signed, read-only Spot balance
// snapshot. It has no trade, cancel, transfer, withdrawal, database or
// scheduler authority.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"btc-agent/internal/okxassets"
)

func main() {
	outDir := os.Getenv("BTC_AGENT_OKX_ASSETS_OUTPUT_DIR")
	baseURL := os.Getenv("BTC_AGENT_OKX_READONLY_BASE_URL")
	if outDir == "" || baseURL == "" {
		log.Fatal("OKX assets output directory and base URL required")
	}
	client := okxassets.NewReadOnlyClient(baseURL, os.Getenv("BTC_AGENT_OKX_READONLY_KEY"), os.Getenv("BTC_AGENT_OKX_READONLY_SECRET"), os.Getenv("BTC_AGENT_OKX_READONLY_PASSPHRASE"), &http.Client{Timeout: 20 * time.Second}, time.Now)
	snapshot, err := client.SpotBalance(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	if err := okxassets.WriteArtifact(outDir, snapshot, time.Now().UTC()); err != nil {
		log.Fatal(err)
	}
}
