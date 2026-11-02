//go:build ignore

// fetchclay downloads clay.h at clayref.ClayCommit into clayref.ClayDir.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/soypat/glay/internal/clayref"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fetchclay:", err)
		os.Exit(1)
	}
}

func run() error {
	url := "https://raw.githubusercontent.com/nicbarker/clay/" + clayref.ClayCommit + "/clay.h"
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	dir := clayref.ClayDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "clay.h"), b, 0o644)
}
