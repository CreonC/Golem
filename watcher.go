package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Plugin represents a Minecraft plugin JAR file
type Plugin struct {
	Path      string    // Full path to the plugin JAR
	Name      string    // Just the filename
	Hash      string    // MD5 hash of the file
	LastCheck time.Time // Last time this plugin was checked
}

// calculateFileHash returns the MD5 hash of a file
func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for hashing: %v", err)
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to calculate hash: %v", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// watchPluginDevelopment starts watching a directory for plugin changes
func watchPluginDevelopment(watchDir string) {
	log.Printf("Starting development watch mode for directory: %s", watchDir)
	
	// Create plugins map with file hashes
	plugins := make(map[string]*Plugin)
	pluginsMutex := &sync.Mutex{}

	// Set up clean exit
	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, os.Interrupt, syscall.SIGTERM)
	
	// Print help message
	fmt.Println("==== Plugin Development Mode ====")
	fmt.Printf("Watching directory: %s\n", watchDir)
	fmt.Printf("Server plugins directory: %s/plugins\n", config.ServerPath)
	fmt.Println("Plugin changes will automatically trigger server restarts")
	fmt.Println("====================================")

	// Ensure server JAR exists and is up to date
	if err := updateServer(); err != nil {
		log.Printf("Failed to update server: %v", err)
		return
	}

	// Scan for initial plugins
	files, err := os.ReadDir(watchDir)
	if err != nil {
		log.Printf("Failed to read watch directory: %v", err)
		return
	}

	// Create initial plugins map with hashes
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), ".jar") {
			pluginPath := filepath.Join(watchDir, file.Name())
			hash, err := calculateFileHash(pluginPath)
			if err != nil {
				log.Printf("Warning: Failed to calculate hash for %s: %v", pluginPath, err)
				continue
			}
			
			plugins[file.Name()] = &Plugin{
				Path:      pluginPath,
				Name:      file.Name(),
				Hash:      hash,
				LastCheck: time.Now(),
			}
			log.Printf("Found plugin: %s (hash: %s)", file.Name(), hash[:8])
		}
	}

	// Copy plugins to server directory
	for _, plugin := range plugins {
		if err := updatePlugin(plugin.Path); err != nil {
			log.Printf("Failed to copy plugin %s: %v", plugin.Name, err)
		}
	}

	// Start server if needed
	if args.AutoStart {
		if err := startServer(); err != nil {
			log.Printf("Failed to start server: %v", err)
			return
		}
	}

	// Start watching for changes in background
	ticker := time.NewTicker(2 * time.Second)
	finished := make(chan struct{})

	// Watch for changes in a separate goroutine
	go func() {
		defer close(finished)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Scan directory for changed plugins
				files, err := os.ReadDir(watchDir)
				if err != nil {
					log.Printf("Failed to read watch directory: %v", err)
					continue
				}

				restartNeeded := false
				pluginsMutex.Lock()

				// Check each JAR file
				for _, file := range files {
					if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), ".jar") {
						pluginPath := filepath.Join(watchDir, file.Name())
						hash, err := calculateFileHash(pluginPath)
						if err != nil {
							log.Printf("Warning: Failed to calculate hash for %s: %v", pluginPath, err)
							continue
						}

						// Check if plugin exists or has changed
						plugin, exists := plugins[file.Name()]
						if !exists {
							// New plugin found
							log.Printf("DEBUG: New plugin detected: %s (hash: %s)", file.Name(), hash[:8])
							plugin = &Plugin{
								Path:      pluginPath,
								Name:      file.Name(),
								Hash:      hash,
								LastCheck: time.Now(),
							}
							plugins[file.Name()] = plugin
							printWithPrompt(fmt.Sprintf("[Golem] New plugin detected: %s", file.Name()))
							
							log.Printf("DEBUG: Copying plugin %s to server plugins directory", file.Name())
							if err := updatePlugin(pluginPath); err != nil {
								log.Printf("Failed to copy plugin %s: %v", file.Name(), err)
								continue
							}
							
							restartNeeded = true
						} else if plugin.Hash != hash {
							// Plugin has changed
							log.Printf("DEBUG: Plugin changed: %s (old hash: %s, new hash: %s)", file.Name(), plugin.Hash[:8], hash[:8])
							printWithPrompt(fmt.Sprintf("[Golem] Plugin changed: %s (hash: %s → %s)", file.Name(), plugin.Hash[:8], hash[:8]))
							plugin.Hash = hash
							plugin.LastCheck = time.Now()
							
							log.Printf("DEBUG: Copying updated plugin %s to server plugins directory", file.Name())
							if err := updatePlugin(pluginPath); err != nil {
								log.Printf("Failed to copy plugin %s: %v", file.Name(), err)
								continue
							}
							
							restartNeeded = true
						} else {
							// Plugin hasn't changed, update last check time
							plugin.LastCheck = time.Now()
						}
					}
				}
				pluginsMutex.Unlock()

				// Restart server if needed
				if restartNeeded {
					printWithPrompt("[Golem] Restarting server to apply plugin changes...")
					if err := restartServer(); err != nil {
						log.Printf("Failed to restart server: %v", err)
					}
				}
				
			case <-exitCh:
				printWithPrompt("\n[Golem] Stopping plugin watcher and server...")
				stopServer()
				return
			}
		}
	}()

	// Block until finished
	<-finished
	fmt.Println("Golem plugin development mode exited")
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
