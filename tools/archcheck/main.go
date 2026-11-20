// Command archcheck enforces Go import boundaries using go list output.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const module = "github.com/herdifirdausss/mamony-go"

type pkg struct {
	ImportPath   string
	Imports      []string
	TestImports  []string
	XTestImports []string
	Error        *struct{ Err string }
}

func main() {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		fatal(err)
	}
	if err := cmd.Start(); err != nil {
		fatal(err)
	}
	dec := json.NewDecoder(out)
	count := 0
	for {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			if err == io.EOF {
				break
			}
			fatal(err)
		}
		if p.Error != nil {
			fatal(fmt.Errorf("go list %s: %s", p.ImportPath, p.Error.Err))
		}
		for _, dep := range append(append(p.Imports, p.TestImports...), p.XTestImports...) {
			if msg := violation(p.ImportPath, dep); msg != "" {
				fmt.Fprintf(os.Stderr, "ARCH VIOLATION: %s -> %s: %s\n", p.ImportPath, dep, msg)
				count++
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		fatal(err)
	}
	if count > 0 {
		fmt.Fprintf(os.Stderr, "%d architecture violations\n", count)
		os.Exit(1)
	}
	fmt.Println("architecture import policy: PASS")
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

func localPath(path string) string {
	prefix := module + "/"
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return ""
}

func capLayer(path string) (capName, layer string) {
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[0] != "internal" || parts[1] != "modules" {
		return "", ""
	}
	capName = parts[2]
	if len(parts) >= 5 && parts[3] == "internal" {
		layer = parts[4]
	}
	return capName, layer
}

func violation(from, to string) string {
	a, b := localPath(from), localPath(to)
	if a == "" || b == "" {
		return ""
	}
	ca, la := capLayer(a)
	cb, lb := capLayer(b)
	if ca != "" && cb != "" && ca != cb {
		return "capabilities must communicate via app wiring"
	}
	if strings.HasPrefix(a, "internal/platform/") && (cb != "" || strings.HasPrefix(b, "internal/app")) {
		return "platform cannot import app or modules"
	}
	if ca != "" && strings.HasPrefix(b, "internal/app") {
		return "capability cannot import app"
	}
	if ca != "" && la == "domain" && (lb == "usecase" || lb == "repository" || lb == "handler" || strings.HasPrefix(b, "internal/platform/")) {
		return "domain must be infrastructure-agnostic"
	}
	if ca != "" && la == "usecase" && (lb == "repository" || lb == "handler") {
		return "usecase cannot import adapters"
	}
	if ca != "" && la == "handler" && lb == "repository" {
		return "handler cannot import repository"
	}
	if cb != "" && lb != "" && strings.HasPrefix(a, "internal/app") {
		return "app cannot bypass capability facade to reach nested internals"
	}
	return ""
}
