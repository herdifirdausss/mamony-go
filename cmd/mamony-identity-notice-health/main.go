// Command mamony-identity-notice-health is a one-shot, read-only,
// thresholded operator probe. It never starts a mail worker, HTTP listener,
// monitoring scheduler, webhook endpoint or production integration.
package main

import (
	"os"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

func main() {
	os.Exit(executeProbe(os.Getenv, os.Args[1:],
		identity.ReadSecurityNoticeHealthAsMonitor, os.Stdout))
}
