// inject-file reads a local file and injects it as a task into a running hive.
//
// Usage:
//
//	go run ./cmd/inject-file design.md
//	go run ./cmd/inject-file design.md --title "Custom Title" --priority high
//	go run ./cmd/inject-file design.md --description "Short summary" --actor Alice
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/transpara-ai/hive/pkg/inject"
)

func defaultActor() string {
	if v := os.Getenv("HIVE_HUMAN"); v != "" {
		return v
	}
	return "Michael"
}

func main() {
	title := flag.String("title", "", "Override title (default: derived from filename)")
	description := flag.String("description", "", "Short summary (file content moves to body field)")
	priority := flag.String("priority", "medium", "Task priority: low, medium, high, critical")
	actor := flag.String("actor", defaultActor(), "Who is injecting the idea (default: $HIVE_HUMAN or Michael)")
	host := flag.String("host", "localhost", "Hive listener host")
	port := flag.String("port", "8081", "Hive listener port")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: inject-file <file> [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Reads a file and injects it as a task into a running hive.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	filePath := flag.Arg(0)
	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(content) == 0 {
		fmt.Fprintln(os.Stderr, "error: file is empty")
		os.Exit(1)
	}

	opts := inject.Options{
		FileContent: string(content),
		SourceFile:  filePath,
		Title:       *title,
		Description: *description,
		Priority:    *priority,
		Actor:       *actor,
	}

	ev, err := inject.BuildEvent(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%s", *host, *port)

	fmt.Fprintf(os.Stderr, "Injecting: %s\n", ev.NodeTitle)
	fmt.Fprintf(os.Stderr, "  File:     %s (%d bytes)\n", filePath, len(content))
	fmt.Fprintf(os.Stderr, "  Priority: %s\n", *priority)
	fmt.Fprintf(os.Stderr, "  Target:   http://%s/event\n", addr)

	if err := inject.Post(ev, addr); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Injected as event %s\n", ev.ID)
}
