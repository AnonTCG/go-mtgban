// Command workflowgen writes .github/workflows/bantool-<game>-<store>.yml,
// one file per row of targets, and removes any bantool workflow no row
// names. Edit the table in targets.go, then run go generate.
package main

//go:generate go run . -dir ../../.github/workflows

import (
	"bytes"
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// workflow is one scheduled bantool run. The file name, the run name and
// the dispatch types are all spelled from Game and Store, and the site
// matches on each of them (cmd/bantool/workflows_test.go).
type workflow struct {
	Game  string
	Store string
	Cron  string
	// Group is the concurrency group shared with the vendor's other runs,
	// or empty for a run that starts whenever its cron fires.
	Group string
	// Affiliate is the repository variable holding the partner code.
	Affiliate string
	// Catalog passes the game's Cardmarket id map.
	Catalog bool
	// Runner is the runs-on label, as a JSON array, for a run too long for
	// a hosted runner; empty runs on ubuntu-latest.
	Runner string
	// Note is written as a comment above the triggers.
	Note string
}

// FileName is the file the workflow is written to.
func (w workflow) FileName() string {
	return "bantool-" + w.Game + "-" + w.Store + ".yml"
}

// CacheDatastore reports whether the run downloads the Magic datastore in
// a job of its own first. The other games read theirs from the bucket, and
// a self-hosted runner cannot restore what a hosted job cached.
func (w workflow) CacheDatastore() bool {
	return w.Game == "magic" && w.Runner == ""
}

// comment prefixes every line of s with "# ".
func comment(s string) string {
	return "# " + strings.ReplaceAll(s, "\n", "\n# ") + "\n"
}

// page is a workflow file. Its delimiters are << and >>, since the file
// itself is full of the ${{ }} expressions GitHub expands.
var page = template.Must(template.New("workflow").
	Delims("<<", ">>").
	Funcs(template.FuncMap{"comment": comment}).
	Parse(`name: <<.Game>> / <<.Store>>

<<with .Note>><<comment .>><<end ->>
on:
  workflow_dispatch: {}
  repository_dispatch:
    types: [<<.Game>>-<<.Store>>, <<.Game>>-all]
  schedule:
    - cron: "<<.Cron>>"

permissions:
  contents: read
<<with .Group>>
concurrency:
  group: <<.>>
  cancel-in-progress: false
  queue: max
<<end>>
jobs:
<<- if .CacheDatastore>>
  cache-datastore:
    uses: ./.github/workflows/cache-file.yml
    with:
      url: ${{ vars.DATASTORE_MAGIC }}
      path: "datastore/"
      game: "magic"
    secrets: inherit
<<end>>
<<- if and (eq .Game "magic") (not .CacheDatastore)>>
  # No cache-datastore job on a self-hosted runner: actions/cache cannot
  # restore there an entry a GitHub-hosted job saved (a documented gap,
  # confirmed live twice against this key). The datastore is read over
  # HTTPS straight from DATASTORE_MAGIC, which simplecloud.Open reads
  # directly, and the id map straight from its bucket, as every game's is.
<<- end>>
  deploy:
    name: ${{ github.workflow }}
<<- if .CacheDatastore>>
    needs: [cache-datastore]
<<- end>>
    uses: ./.github/workflows/run-bantool.yml
    with:
      store: <<.Store>>
      game: "<<.Game>>"
<<- with .Affiliate>>
      affiliate: ${{ vars.<<.>> }}
<<- end>>
<<- if .CacheDatastore>>
      datastore-filepath: ${{ needs.cache-datastore.outputs.filepath }}
      datastore-cache-key: ${{ needs.cache-datastore.outputs.key }}
<<- else if eq .Game "magic">>
      datastore-filepath: ${{ vars.DATASTORE_MAGIC }}
<<- else>>
      datastore-filepath: b2://mtgban-datastore/<<.Game>>/<<.Game>>.json.xz
<<- end>>
<<- if .Catalog>>
      cardmarket-catalog-filepath: b2://mtgban-datastore/<<.Game>>/cardmarket_catalog.json.xz
<<- end>>
<<- with .Runner>>
      runs-on: '<<.>>'
<<- end>>
    secrets: inherit
`))

// write renders every target into dir and removes the bantool workflows
// there that no target names.
func write(dir string) error {
	want := map[string]bool{}
	for _, w := range targets {
		name := w.FileName()
		if want[name] {
			return errors.New("two targets write " + name)
		}
		want[name] = true

		var buf bytes.Buffer
		err := page.Execute(&buf, w)
		if err != nil {
			return err
		}
		err = os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o644)
		if err != nil {
			return err
		}
	}

	paths, err := filepath.Glob(filepath.Join(dir, "bantool-*.yml"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		if want[filepath.Base(path)] {
			continue
		}
		err := os.Remove(path)
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	dir := flag.String("dir", "", "directory to write the workflows to (required)")
	flag.Parse()
	if *dir == "" {
		log.Fatalln("-dir is required: it is the directory whose bantool workflows are replaced")
	}

	err := write(*dir)
	if err != nil {
		log.Fatalln(err)
	}
}
