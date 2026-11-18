package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/takutakahashi/archscore/internal/analyzer"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "discover" {
		discover(os.Args[2:])
		return
	}
	analyze(os.Args[1:])
}

func analyze(args []string) {
	flags := flag.NewFlagSet("archscore", flag.ExitOnError)
	root := flags.String("root", ".", "repository root")
	configPath := flags.String("config", "archscore.json", "configuration file")
	pretty := flags.Bool("pretty", true, "pretty-print JSON")
	_ = flags.Parse(args)

	cfg, err := analyzer.LoadConfig(*configPath)
	if err != nil {
		fatal(err)
	}
	report, err := analyzer.Analyze(*root, cfg)
	if err != nil {
		fatal(err)
	}
	var out []byte
	if *pretty {
		out, err = json.MarshalIndent(report, "", "  ")
	} else {
		out, err = json.Marshal(report)
	}
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(out))
	if report.Gate == "fail" {
		os.Exit(2)
	}
}

func discover(args []string) {
	flags := flag.NewFlagSet("archscore discover", flag.ExitOnError)
	root := flags.String("root", ".", "repository root")
	minClusters := flags.Int("min-clusters", 8, "smallest candidate boundary count")
	maxClusters := flags.Int("max-clusters", 30, "largest candidate boundary count")
	history := flags.Int("history", 200, "maximum commits to analyze")
	minCochange := flags.Int("min-cochange", 3, "minimum shared commits for change affinity")
	maxBoundarySize := flags.Int("max-boundary-size", 12, "maximum source units in a suggested boundary")
	maxBoundaryFiles := flags.Int("max-boundary-files", 100, "maximum source files in a suggested boundary")
	pretty := flags.Bool("pretty", true, "pretty-print JSON")
	_ = flags.Parse(args)
	report, err := analyzer.Discover(*root, analyzer.DiscoverConfig{
		MinClusters: *minClusters, MaxClusters: *maxClusters, HistoryCommits: *history,
		MinCochange: *minCochange, MaxBoundarySize: *maxBoundarySize, MaxBoundaryFiles: *maxBoundaryFiles,
		Exclude: []string{"**/*_test.go", "**/*.test.ts", "**/*.test.tsx"},
	})
	if err != nil {
		fatal(err)
	}
	printJSON(report, *pretty)
}

func printJSON(value any, pretty bool) {
	var out []byte
	var err error
	if pretty {
		out, err = json.MarshalIndent(value, "", "  ")
	} else {
		out, err = json.Marshal(value)
	}
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(out))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "archscore:", err)
	os.Exit(1)
}
