package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/minio/madmin-go/v3"
)

type aliasConfig struct {
	URL       string `json:"url"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
	API       string `json:"api"`
	Path      string `json:"path"`
}

type mcConfig struct {
	Version string                 `json:"version"`
	Aliases map[string]aliasConfig `json:"aliases"`
}

func loadAlias(alias, configDir string) (aliasConfig, error) {
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return aliasConfig{}, fmt.Errorf("get home dir: %w", err)
		}
		configDir = filepath.Join(homeDir, ".mc")
	}

	data, err := os.ReadFile(filepath.Join(configDir, "config.json"))
	if err != nil {
		return aliasConfig{}, fmt.Errorf("read %s: %w", configDir, err)
	}

	var cfg mcConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return aliasConfig{}, fmt.Errorf("parse config: %w", err)
	}

	ac, ok := cfg.Aliases[alias]
	if !ok {
		return aliasConfig{}, fmt.Errorf("alias %q not found in %s", alias, configDir)
	}
	return ac, nil
}

func newAdminClient(ac aliasConfig) (*madmin.AdminClient, error) {
	u, err := url.Parse(ac.URL)
	if err != nil {
		return nil, fmt.Errorf("parse URL %q: %w", ac.URL, err)
	}

	secure := strings.EqualFold(u.Scheme, "https")
	client, err := madmin.New(u.Host, ac.AccessKey, ac.SecretKey, secure)
	if err != nil {
		return nil, err
	}

	if secure {
		client.SetCustomTransport(&http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		})
	}

	return client, nil
}

func main() {
	configDir := flag.String("config-dir", "", "path to mc config directory (default: ~/.mc)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <alias>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	alias := flag.Arg(0)

	ac, err := loadAlias(alias, *configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	client, err := newAdminClient(ac)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating admin client: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pools, err := client.ListPoolsStatus(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing pool status: %v\n", err)
		os.Exit(1)
	}

	hasDraining := false
	for _, pool := range pools {
		d := pool.Decommission
		if d == nil || d.StartTime.IsZero() {
			continue
		}

		if d.Complete || d.Failed || d.Canceled {
			continue
		}

		hasDraining = true

		// StartSize/CurrentSize = free bytes at decom start / now.
		// As data moves off pool, free space increases: CurrentSize > StartSize.
		// initialUsed = data that needs to move off.
		// bytesFreed = free space gained so far.
		initialUsed := d.TotalSize - d.StartSize
		bytesFreed := d.CurrentSize - d.StartSize
		usedNow := d.TotalSize - d.CurrentSize

		fmt.Printf("Pool #%d: %s\n", pool.ID+1, pool.CmdLine)

		elapsed := time.Since(d.StartTime)
		fmt.Printf("  Started: %s (%s ago)\n", d.StartTime.Format(time.RFC3339), humanize.RelTime(d.StartTime, time.Now(), "", ""))

		if bytesFreed > 0 && initialUsed > 0 && elapsed.Seconds() > 10 {
			progress := float64(bytesFreed) / float64(initialUsed)
			speed := float64(bytesFreed) / elapsed.Seconds()

			fmt.Printf("  Progress: %s / %s freed (%.1f%%)\n",
				humanize.IBytes(uint64(bytesFreed)),
				humanize.IBytes(uint64(initialUsed)),
				progress*100)
			fmt.Printf("  Current usage: %s / %s (%.1f%%)\n",
				humanize.IBytes(uint64(usedNow)),
				humanize.IBytes(uint64(d.TotalSize)),
				100*float64(usedNow)/float64(d.TotalSize))
			fmt.Printf("  Speed: %s/sec\n", humanize.IBytes(uint64(speed)))

			if progress > 0 && progress < 1.0 {
				totalEstimated := elapsed.Seconds() / progress
				etaSeconds := totalEstimated - elapsed.Seconds()
				etaDuration := time.Duration(etaSeconds) * time.Second
				etaTime := time.Now().Add(etaDuration)
				fmt.Printf("  ETA: %s (%s remaining)\n",
					etaTime.Format(time.RFC3339),
					formatDuration(etaDuration))
			}
		} else {
			fmt.Println("  Decommissioning is starting, ETA not yet available...")
		}
		fmt.Println()
	}

	if !hasDraining {
		fmt.Println("No pools are currently being decommissioned.")
	}
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if mins > 0 {
		parts = append(parts, fmt.Sprintf("%dm", mins))
	}
	if len(parts) == 0 {
		return "< 1m"
	}
	return strings.Join(parts, " ")
}
