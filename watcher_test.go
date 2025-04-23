package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPluginHashDetection tests that the plugin hash detection works correctly
func TestPluginHashDetection(t *testing.T) {
	// Create temporary test directories
	tempWatchDir, err := ioutil.TempDir("", "golem-watch-test")
	if err != nil {
		t.Fatalf("Failed to create temp watch dir: %v", err)
	}
	defer os.RemoveAll(tempWatchDir)

	tempServerDir, err := ioutil.TempDir("", "golem-server-test")
	if err != nil {
		t.Fatalf("Failed to create temp server dir: %v", err)
	}
	defer os.RemoveAll(tempServerDir)

	// Create plugins directory in server dir
	tempPluginsDir := filepath.Join(tempServerDir, "plugins")
	if err := os.MkdirAll(tempPluginsDir, 0755); err != nil {
		t.Fatalf("Failed to create plugins dir: %v", err)
	}

	// Create a mock plugin file
	pluginPath := filepath.Join(tempWatchDir, "TestPlugin.jar")
	testContent := []byte("Test plugin content version 1")
	if err := ioutil.WriteFile(pluginPath, testContent, 0644); err != nil {
		t.Fatalf("Failed to write test plugin: %v", err)
	}

	// Calculate hash of the plugin
	originalHash, err := calculateFileHash(pluginPath)
	if err != nil {
		t.Fatalf("Failed to calculate plugin hash: %v", err)
	}

	// Test hash calculation
	t.Run("Hash calculation works", func(t *testing.T) {
		hash, err := calculateFileHash(pluginPath)
		if err != nil {
			t.Fatalf("Failed to calculate hash: %v", err)
		}
		if hash != originalHash {
			t.Errorf("Expected hash %s, got %s", originalHash, hash)
		}
	})

	// Test hash changes when content changes
	t.Run("Hash changes when content changes", func(t *testing.T) {
		// Modify the plugin file
		newContent := []byte("Test plugin content version 2 - modified")
		if err := ioutil.WriteFile(pluginPath, newContent, 0644); err != nil {
			t.Fatalf("Failed to write modified plugin: %v", err)
		}

		// Get new hash
		newHash, err := calculateFileHash(pluginPath)
		if err != nil {
			t.Fatalf("Failed to calculate new hash: %v", err)
		}

		// Hash should change
		if newHash == originalHash {
			t.Errorf("Hash did not change after file content was modified")
		}
	})
}

// TestPluginScanner tests that the plugin scanner correctly detects new and modified plugins
func TestPluginScanner(t *testing.T) {
	// Create temporary test directories
	tempWatchDir, err := ioutil.TempDir("", "golem-watch-test")
	if err != nil {
		t.Fatalf("Failed to create temp watch dir: %v", err)
	}
	defer os.RemoveAll(tempWatchDir)

	// Mock config
	origServerPath := config.ServerPath
	config.ServerPath = filepath.Join(tempWatchDir, "server")
	defer func() { config.ServerPath = origServerPath }()

	// Create plugins directory
	pluginsDir := filepath.Join(config.ServerPath, "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		t.Fatalf("Failed to create plugins dir: %v", err)
	}

	// Create a test plugin
	plugin1Path := filepath.Join(tempWatchDir, "TestPlugin1.jar")
	if err := ioutil.WriteFile(plugin1Path, []byte("Test plugin 1"), 0644); err != nil {
		t.Fatalf("Failed to write test plugin: %v", err)
	}

	// Set up plugins map for tracking
	plugins := make(map[string]*Plugin)

	// Test initial scan detects plugin
	t.Run("Initial scan detects plugin", func(t *testing.T) {
		files, _ := os.ReadDir(tempWatchDir)
		for _, file := range files {
			if !file.IsDir() && filepath.Ext(file.Name()) == ".jar" {
				path := filepath.Join(tempWatchDir, file.Name())
				hash, _ := calculateFileHash(path)
				plugins[file.Name()] = &Plugin{
					Path:      path,
					Name:      file.Name(),
					Hash:      hash,
					LastCheck: time.Now(),
				}
			}
		}

		if len(plugins) != 1 {
			t.Errorf("Expected 1 plugin, found %d", len(plugins))
		}

		if plugin, exists := plugins["TestPlugin1.jar"]; !exists {
			t.Errorf("Failed to detect TestPlugin1.jar")
		} else if plugin.Name != "TestPlugin1.jar" {
			t.Errorf("Expected plugin name TestPlugin1.jar, got %s", plugin.Name)
		}
	})

	// Test detecting a modified plugin
	t.Run("Detects modified plugin", func(t *testing.T) {
		// Get original hash
		originalPlugin := plugins["TestPlugin1.jar"]
		originalHash := originalPlugin.Hash

		// Modify plugin
		newContent := []byte("Test plugin 1 - MODIFIED")
		if err := ioutil.WriteFile(plugin1Path, newContent, 0644); err != nil {
			t.Fatalf("Failed to write modified plugin: %v", err)
		}

		// Scan again (simulate detection)
		newHash, _ := calculateFileHash(plugin1Path)
		
		// Verify hash changed
		if newHash == originalHash {
			t.Errorf("Plugin hash should have changed after modification")
		}
		
		// Update plugin in map (simulate what watcher would do)
		originalPlugin.Hash = newHash
		
		// Verify the update happened
		if plugins["TestPlugin1.jar"].Hash != newHash {
			t.Errorf("Plugin hash was not updated correctly")
		}
	})

	// Test detecting a new plugin
	t.Run("Detects new plugin", func(t *testing.T) {
		// Create a new plugin
		plugin2Path := filepath.Join(tempWatchDir, "TestPlugin2.jar")
		if err := ioutil.WriteFile(plugin2Path, []byte("Test plugin 2"), 0644); err != nil {
			t.Fatalf("Failed to write new plugin: %v", err)
		}

		// Simulate detection
		files, _ := os.ReadDir(tempWatchDir)
		for _, file := range files {
			if !file.IsDir() && filepath.Ext(file.Name()) == ".jar" {
				if _, exists := plugins[file.Name()]; !exists {
					// New plugin found
					path := filepath.Join(tempWatchDir, file.Name())
					hash, _ := calculateFileHash(path)
					plugins[file.Name()] = &Plugin{
						Path:      path,
						Name:      file.Name(),
						Hash:      hash,
						LastCheck: time.Now(),
					}
				}
			}
		}

		// Should now have 2 plugins
		if len(plugins) != 2 {
			t.Errorf("Expected 2 plugins, found %d", len(plugins))
		}

		// The new plugin should be detected
		if _, exists := plugins["TestPlugin2.jar"]; !exists {
			t.Errorf("Failed to detect new plugin TestPlugin2.jar")
		}
	})
}

// TestUpdatePlugin tests that plugins are correctly copied to the server directory
func TestUpdatePlugin(t *testing.T) {
	// Create temporary test directories
	tempWatchDir, err := ioutil.TempDir("", "golem-watch-test")
	if err != nil {
		t.Fatalf("Failed to create temp watch dir: %v", err)
	}
	defer os.RemoveAll(tempWatchDir)

	// Mock config
	origServerPath := config.ServerPath
	config.ServerPath = filepath.Join(tempWatchDir, "server")
	defer func() { config.ServerPath = origServerPath }()

	// Create plugins directory
	pluginsDir := filepath.Join(config.ServerPath, "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		t.Fatalf("Failed to create plugins dir: %v", err)
	}

	// Create a test plugin
	plugin1Path := filepath.Join(tempWatchDir, "TestPlugin1.jar")
	testContent := []byte("Test plugin content for copying test")
	if err := ioutil.WriteFile(plugin1Path, testContent, 0644); err != nil {
		t.Fatalf("Failed to write test plugin: %v", err)
	}

	// Test plugin copying
	t.Run("Copies plugin to server directory", func(t *testing.T) {
		// Run the updatePlugin function
		err := updatePlugin(plugin1Path)
		if err != nil {
			t.Fatalf("Failed to update plugin: %v", err)
		}

		// Check if the plugin was copied to the server plugins directory
		destPath := filepath.Join(pluginsDir, "TestPlugin1.jar")
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			t.Errorf("Plugin was not copied to server plugins directory")
		}

		// Verify content was copied correctly
		destContent, err := ioutil.ReadFile(destPath)
		if err != nil {
			t.Fatalf("Failed to read copied plugin: %v", err)
		}

		if string(destContent) != string(testContent) {
			t.Errorf("Plugin content was not copied correctly")
		}
	})

	// Test plugin update overrides previous version
	t.Run("Updates existing plugin", func(t *testing.T) {
		// Modify the source plugin
		newContent := []byte("Modified test plugin content")
		if err := ioutil.WriteFile(plugin1Path, newContent, 0644); err != nil {
			t.Fatalf("Failed to write modified plugin: %v", err)
		}

		// Run the updatePlugin function again
		err := updatePlugin(plugin1Path)
		if err != nil {
			t.Fatalf("Failed to update plugin: %v", err)
		}

		// Read the copied plugin
		destPath := filepath.Join(pluginsDir, "TestPlugin1.jar")
		destContent, err := ioutil.ReadFile(destPath)
		if err != nil {
			t.Fatalf("Failed to read copied plugin: %v", err)
		}

		// Verify the plugin was updated
		if string(destContent) != string(newContent) {
			t.Errorf("Plugin was not updated correctly")
		}
	})
}

// Helper function to create a test JAR file
func createTestJar(path, content string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create test jar: %v", err)
	}
	defer file.Close()
	
	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("failed to write content to jar: %v", err)
	}
	
	return nil
}
