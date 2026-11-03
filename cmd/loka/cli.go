package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/sportifseif5-coder/loka---reviewer/internal/advisory"
	"github.com/sportifseif5-coder/loka---reviewer/internal/emit"
	"github.com/sportifseif5-coder/loka---reviewer/internal/model"
	"github.com/sportifseif5-coder/loka---reviewer/internal/portable"
	"github.com/sportifseif5-coder/loka---reviewer/internal/precision"
	"github.com/sportifseif5-coder/loka---reviewer/internal/store"
)

func runEmit(args []string) {
	fs := flag.NewFlagSet("emit", flag.ExitOnError)
	reviewJSON := fs.String("review", "", "path to a review JSON file")
	targetURL := fs.String("target", "", "GitHub pull or GitLab merge-request URL")
	dry := fs.Bool("dry-run", true, "print comments without posting (always the safe default)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *reviewJSON == "" || *targetURL == "" {
		fmt.Fprintln(os.Stderr, "usage: loka emit -review FILE.json -target URL")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*reviewJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read review: %v\n", err)
		os.Exit(1)
	}
	var res model.ReviewResult
	if err := json.Unmarshal(raw, &res); err != nil {
		fmt.Fprintf(os.Stderr, "parse review: %v\n", err)
		os.Exit(1)
	}
	t, err := emit.ParseTarget(*targetURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "target: %v\n", err)
		os.Exit(1)
	}
	e := emit.Emitter{DryRun: *dry}
	n, err := e.Emit(context.Background(), t, &res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "emit: %v\n", err)
		os.Exit(1)
	}
	for _, f := range res.Findings {
		if f.Location.File == "" {
			continue
		}
		fmt.Printf("%s:%d\n%s\n---\n", f.Location.File, f.Location.LineStart, emit.Format(f, string(t.Host)))
	}
	fmt.Printf("emit-only: %d comment(s) for %s/%s#%d (dry-run=%v, merge=never)\n", n, t.Owner, t.Repo, t.Number, *dry)
}

func runExport(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	dbPath := fs.String("db", "", "path to the loka database")
	out := fs.String("out", "", "destination directory")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *out == "" {
		fmt.Fprintln(os.Stderr, "usage: loka export [-db PATH] -out DIR")
		os.Exit(2)
	}
	db := *dbPath
	if db == "" {
		db = defaultDBPath()
	}
	s, err := store.Open(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "store: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()
	if err := portable.Export(context.Background(), s, *out); err != nil {
		fmt.Fprintf(os.Stderr, "export: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("exported to %s\n", *out)
}

func runImport(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	dbPath := fs.String("db", "", "path to the loka database")
	from := fs.String("from", "", "export directory")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *from == "" {
		fmt.Fprintln(os.Stderr, "usage: loka import [-db PATH] -from DIR")
		os.Exit(2)
	}
	db := *dbPath
	if db == "" {
		db = defaultDBPath()
	}
	s, err := store.Open(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "store: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()
	if err := portable.Import(context.Background(), s, *from); err != nil {
		fmt.Fprintf(os.Stderr, "import: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("imported from %s\n", *from)
}

func runAdvisoryUpdate(args []string) {
	fs := flag.NewFlagSet("advisory-update", flag.ExitOnError)
	from := fs.String("from", "", "local advisory JSON file")
	to := fs.String("to", "", "destination path")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *from == "" || *to == "" {
		fmt.Fprintln(os.Stderr, "usage: loka advisory-update -from FILE -to FILE")
		os.Exit(2)
	}
	if err := advisory.Update(*from, *to); err != nil {
		fmt.Fprintf(os.Stderr, "advisory-update: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("updated local advisory db %s\n", *to)
}

func runPrecision(args []string) {
	fs := flag.NewFlagSet("precision", flag.ExitOnError)
	dir := fs.String("corpus", "internal/precision/testdata/corpus", "corpus directory")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	rep, err := precision.Evaluate(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "precision: %v\n", err)
		os.Exit(1)
	}
	out, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
	if !rep.Pass {
		os.Exit(1)
	}
}
