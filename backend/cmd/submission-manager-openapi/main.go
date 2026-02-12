package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gateway/submissionmanagerapi"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		exitf("%v", err)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("submission-manager-openapi", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	out := fs.String("out", "", "write output to file (default stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	payload, err := submissionmanagerapi.MarshalOpenAPIJSON()
	if err != nil {
		return fmt.Errorf("generate OpenAPI JSON: %w", err)
	}
	payload = append(payload, '\n')

	if *out == "" {
		if _, err := stdout.Write(payload); err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
		return nil
	}

	path := filepath.Clean(*out)
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func exitf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
