package main

import (
	"context"
	"cullsnap/internal/hfclient"
	"cullsnap/internal/vlm"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
)

func main() {
	var (
		repo     = flag.String("repo", "", "HF repo id (e.g. mlx-community/gemma-4-E4B-it-4bit)")
		revision = flag.String("revision", "main", "HF revision (branch, tag, or commit SHA)")
		engine   = flag.String("engine", "", "engine name (mlx_vlm | vllm_mlx | empty for gguf)")
		name     = flag.String("name", "", "model name (e.g. gemma-4-e4b-it)")
		variant  = flag.String("variant", "", "variant (e.g. mlx-4bit)")
		format   = flag.String("format", "mlx", "format (mlx | gguf)")
		backend  = flag.String("backend", "mlx", "backend (mlx | llamacpp)")
		minTier  = flag.String("min-tier", "capable", "min tier (basic | capable | maximum)")
		ramUsage = flag.String("ram-usage", "0", "approximate RAM usage in bytes")
		token    = flag.String("hf-token", os.Getenv("HF_TOKEN"), "HF auth token (optional)")
	)
	flag.Parse()
	if *repo == "" || *name == "" || *variant == "" {
		fatal("repo, name, variant required")
	}

	c := hfclient.New(*token)
	entries, commit, err := c.FetchTree(context.Background(), *repo, *revision)
	if err != nil {
		fatal(err.Error())
	}
	if commit == "" {
		fatal("server did not return X-Repo-Commit")
	}
	perFile := make([]hfclient.FileEntry, 0, len(entries))
	var total int64
	for _, e := range entries {
		if e.Path == ".gitattributes" || e.Path == "README.md" {
			continue
		}
		perFile = append(perFile, hfclient.FileEntry{
			Path: e.Path, Size: e.Size,
			SHA256: e.SHA256, SHA1: e.SHA1, IsLFS: e.IsLFS,
		})
		total += e.Size
	}

	var tier vlm.HardwareTier
	switch *minTier {
	case "basic":
		tier = vlm.TierBasic
	case "capable":
		tier = vlm.TierCapable
	case "maximum":
		tier = vlm.TierMaximum
	default:
		fatal("invalid --min-tier")
	}
	ram, err := strconv.ParseInt(*ramUsage, 10, 64)
	if err != nil {
		fatal("invalid --ram-usage")
	}

	m := vlm.ModelManifest{
		Name:          *name,
		Variant:       *variant,
		Format:        *format,
		Engine:        vlm.ServerEngine(*engine),
		Backend:       *backend,
		Repo:          *repo,
		CommitSHA:     commit,
		AllowPatterns: []string{"*.json", "*.safetensors*", "*.jinja", "*.model", "tokenizer.model"},
		PerFile:       perFile,
		SizeBytes:     total,
		RAMUsage:      ram,
		MinTier:       tier,
		Available:     true,
	}
	out, _ := json.MarshalIndent(m, "", "  ")
	fmt.Println(string(out))
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "vlm-manifest:", msg)
	os.Exit(1)
}
