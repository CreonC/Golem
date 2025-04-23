package main

import (
	"io"
	"log"
	"os"

	"github.com/alexflint/go-arg"
)

// ServerType represents the type of Minecraft server
type ServerType string

const (
	Vanilla ServerType = "vanilla"
	Paper   ServerType = "paper"
	Purpur  ServerType = "purpur"
)

// Config represents the server configuration
type Config struct {
	ServerType              ServerType `json:"serverType"`
	ServerVersion           string     `json:"serverVersion"`
	BuildNumber             int        `json:"buildNumber,omitempty"`
	JavaPath                string     `json:"javaPath"`
	MinRAM                  string     `json:"minRam"`
	MaxRAM                  string     `json:"maxRam"`
	ServerPath              string     `json:"serverPath"`
	AllowExperimentalBuilds bool       `json:"allowExperimentalBuilds"`
}

// Args represents command-line arguments
type Args struct {
	Config    string `arg:"--config" help:"Path to config file"`
	Watch     string `arg:"--watch" help:"Path to plugin development directory to watch"`
	AutoStart bool   `arg:"--auto-start" help:"Automatically start server after update"`
}

var config Config
var args Args
var serverProcess *os.Process
var serverStdin io.WriteCloser

func main() {
	log.Println("Golem - The minecraft server manager and watcher  |  version 0.1.0")
	arg.MustParse(&args)

	// Load configuration
	if args.Config == "" {
		args.Config = "golem-config.json"
	}

	if err := loadConfig(args.Config); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Handle development watch mode
	if args.Watch != "" {
		log.Printf("Starting development watch mode for directory: %s", args.Watch)
		watchPluginDevelopment(args.Watch)
		return
	}

	// Normal update mode
	if err := updateServer(); err != nil {
		log.Printf("Failed to update server: %v", err)
		log.Println("If you want to run non-stable builds, set 'allowExperimentalBuilds' to true in the config file.")
		os.Exit(1)
	}

	if args.AutoStart {
		if err := startServer(); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
		
		// Block here to keep Golem running as a middleman
		// This will only exit if the process is killed or an exit command is given
		waitForExit := make(chan struct{})
		<-waitForExit // This blocks forever until channel is closed
	}
}
