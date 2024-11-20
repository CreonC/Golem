package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func watchPluginDevelopment(watchDir string) {
	log.Printf("Starting development watch mode for directory: %s", watchDir)

	// Ensure server JAR exists and is up to date
	if err := updateServer(); err != nil {
		log.Printf("Failed to update server: %v", err)
		return
	}

	// Start server initially if auto-start is enabled
	if args.AutoStart {
		if err := startServer(); err != nil {
			log.Printf("Failed to start server: %v", err)
			return
		}
	}

	// Create initial map of file modification times
	lastModified := make(map[string]time.Time)
	files, err := os.ReadDir(watchDir)
	if err != nil {
		log.Printf("Failed to read watch directory: %v", err)
		return
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), ".jar") {
			info, err := file.Info()
			if err != nil {
				continue
			}
			lastModified[file.Name()] = info.ModTime()
		}
	}

	// Watch for changes
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		files, err := os.ReadDir(watchDir)
		if err != nil {
			log.Printf("Failed to read watch directory: %v", err)
			continue
		}

		restartNeeded := false
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), ".jar") {
				info, err := file.Info()
				if err != nil {
					continue
				}

				lastMod, exists := lastModified[file.Name()]
				if !exists || lastMod != info.ModTime() {
					pluginPath := filepath.Join(watchDir, file.Name())
					log.Printf("Modified plugin detected: %s", pluginPath)

					if err := updatePlugin(pluginPath); err != nil {
						log.Printf("Failed to update plugin: %v", err)
						continue
					}

					lastModified[file.Name()] = info.ModTime()
					restartNeeded = true
				}
			}
		}

		if restartNeeded {
			log.Println("Restarting server to apply plugin changes...")
			if err := restartServer(); err != nil {
				log.Printf("Failed to restart server: %v", err)
			}
		}
	}
}

func updatePlugin(pluginPath string) error {
	// Copy new plugin to server's plugin directory
	destPath := filepath.Join(config.ServerPath, "plugins", filepath.Base(pluginPath))
	log.Printf("Copying plugin %s to %s", pluginPath, destPath)

	// Create plugins directory if it doesn't exist
	if err := os.MkdirAll(filepath.Join(config.ServerPath, "plugins"), 0755); err != nil {
		return fmt.Errorf("failed to create plugins directory: %v", err)
	}

	return copyFile(pluginPath, destPath)
}
