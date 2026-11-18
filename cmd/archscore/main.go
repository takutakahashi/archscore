package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/takutakahashi/archscore/internal/analyzer"
)

func main() {
	root := flag.String("root", ".", "repository root")
	configPath := flag.String("config", "archscore.json", "configuration file")
	pretty := flag.Bool("pretty", true, "pretty-print JSON")
	flag.Parse()

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

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "archscore:", err)
	os.Exit(1)
}
