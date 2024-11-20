package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/schollz/progressbar/v3"
)

// ServerType represents the type of Minecraft server
type ServerType string

const (
	Vanilla ServerType = "vanilla"
	Paper   ServerType = "paper"
	Spigot  ServerType = "spigot"
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
	}
}

func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config file: %v", err)
	}

	return nil
}

func updateServer() error {
	// Create server directory if it doesn't exist
	if err := os.MkdirAll(config.ServerPath, 0755); err != nil {
		return fmt.Errorf("failed to create server directory: %v", err)
	}

	// Update server based on type
	switch config.ServerType {
	case Paper, Purpur:
		return updatePaperLike()
	case Spigot:
		return fmt.Errorf("spigot server type not implemented yet")
	case Vanilla:
		return fmt.Errorf("vanilla server type not implemented yet")
	default:
		return fmt.Errorf("unknown server type: %s", config.ServerType)
	}
}

type PaperAPIVersions struct {
	ProjectID   string   `json:"project_id"`
	ProjectName string   `json:"project_name"`
	Versions    []string `json:"versions"`
}

type PaperAPIBuild struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Version     string `json:"version"`
	Build       int    `json:"build"`
	Time        string `json:"time"`
	Channel     string `json:"channel"`
	Promoted    bool   `json:"promoted"`
	Changes     []struct {
		Commit  string `json:"commit"`
		Summary string `json:"summary"`
		Message string `json:"message"`
	} `json:"changes"`
	Downloads struct {
		Application struct {
			Name   string `json:"name"`
			Sha256 string `json:"sha256"`
		} `json:"application"`
	} `json:"downloads"`
}

type PaperAPIBuilds struct {
	ProjectID   string          `json:"project_id"`
	ProjectName string          `json:"project_name"`
	Version     string          `json:"version"`
	Builds      []PaperAPIBuild `json:"builds"`
}

func verifyFileHash(filepath string, expectedHash string) error {
	// Open file for reading
	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("failed to open file for verification: %v", err)
	}
	defer file.Close()

	// Calculate hash
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("failed to calculate file hash: %v", err)
	}

	calculatedHash := hex.EncodeToString(hash.Sum(nil))
	if calculatedHash != expectedHash {
		return fmt.Errorf("hash verification failed. Expected: %s, Got: %s",
			expectedHash, calculatedHash)
	}

	return nil
}

func updatePaperLike() error {
	projectName := strings.ToLower(string(config.ServerType))

	// Get latest version
	versionsURL := fmt.Sprintf("https://api.papermc.io/v2/projects/%s", projectName)
	versions := &PaperAPIVersions{}
	if err := fetchJSON(versionsURL, versions); err != nil {
		return fmt.Errorf("failed to fetch versions: %v", err)
	}

	latestVersion := versions.Versions[len(versions.Versions)-1]
	currentVersion := config.ServerVersion

	// Check if major version update is needed
	if latestVersion != currentVersion {
		log.Printf("New major version available: %s (current: %s)", latestVersion, currentVersion)
		log.Println("Major version updates may break compatibility. Please update the config file manually if you want to upgrade.")
		return nil
	}

	// Get latest build
	buildsURL := fmt.Sprintf("https://api.papermc.io/v2/projects/%s/versions/%s/builds", projectName, currentVersion)
	log.Println("Fetching builds... (build number: " + fmt.Sprint(config.BuildNumber) + ", fullURL: " + buildsURL + ")")
	builds := &PaperAPIBuilds{}
	if err := fetchJSON(buildsURL, builds); err != nil {
		return fmt.Errorf("failed to fetch builds: %v", err)
	}

	var latestBuild int
	for i := len(builds.Builds) - 1; i >= 0; i-- {
		build := builds.Builds[i]
		if config.AllowExperimentalBuilds || args.Watch != "" || build.Channel == "default" {
			latestBuild = build.Build
			break
		}
	}

	if latestBuild == 0 {
		return fmt.Errorf("no suitable or stable build found for version %s", currentVersion)
	}

	serverJar := filepath.Join(config.ServerPath, "server.jar")
	jarExists := true
	if _, err := os.Stat(serverJar); os.IsNotExist(err) {
		jarExists = false
	}

	if latestBuild <= config.BuildNumber && jarExists {
		log.Printf("Already on latest build %d", config.BuildNumber)
		return nil
	}

	// Fetch specific build information
	buildURL := fmt.Sprintf("https://api.papermc.io/v2/projects/%s/versions/%s/builds/%d",
		projectName, currentVersion, latestBuild)
	buildInfo := &PaperAPIBuild{}
	if err := fetchJSON(buildURL, buildInfo); err != nil {
		return fmt.Errorf("failed to fetch build info: %v", err)
	}

	// Print build changes
	log.Printf("Build %d changes:", latestBuild)
	for _, change := range buildInfo.Changes {
		log.Printf("- %s", change.Summary)
	}

	if buildInfo.Channel != "default" {
		log.Printf("Warning: Using %s build %d. This build may contain bugs or unfinished features!",
			buildInfo.Channel, latestBuild)
	}

	// Download new version using information from the build API
	downloadURL := fmt.Sprintf("https://api.papermc.io/v2/projects/%s/versions/%s/builds/%d/downloads/%s",
		projectName, currentVersion, latestBuild, buildInfo.Downloads.Application.Name)

	log.Printf("Downloading %s (SHA256: %s)", buildInfo.Downloads.Application.Name,
		buildInfo.Downloads.Application.Sha256)

	// Create a temporary file for downloading
	tmpFile := serverJar + ".tmp"
	if err := downloadFile(downloadURL, tmpFile); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to download new server jar: %v", err)
	}

	// Verify the downloaded file
	if err := verifyFileHash(tmpFile, buildInfo.Downloads.Application.Sha256); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to verify download: %v", err)
	}

	// Close any open handles to the old file
	if jarExists {
		runtime.GC() // Help release any file handles ???
	}

	// Replace the old jar with the new one
	if err := os.Rename(tmpFile, serverJar); err != nil {
		// If rename fails, try copy and delete
		srcFile, err := os.Open(tmpFile)
		if err != nil {
			os.Remove(tmpFile)
			return fmt.Errorf("failed to open temporary file: %v", err)
		}
		defer srcFile.Close()

		dstFile, err := os.Create(serverJar)
		if err != nil {
			os.Remove(tmpFile)
			return fmt.Errorf("failed to create new server jar: %v", err)
		}
		defer dstFile.Close()

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			os.Remove(tmpFile)
			return fmt.Errorf("failed to copy server jar: %v", err)
		}

		// Close files explicitly before removing
		srcFile.Close()
		dstFile.Close()
		os.Remove(tmpFile)
	}

	// Update config with new build number
	config.BuildNumber = latestBuild
	if err := saveConfig(args.Config); err != nil {
		return fmt.Errorf("failed to save config: %v", err)
	}

	log.Printf("Successfully updated %s to build %d", config.ServerType, latestBuild)
	return acceptEULA()
}

func updateSpigot() error {
	// Implementation for Spigot update logic
	return nil
}

func updateVanilla() error {
	// Implementation for Vanilla update logic
	return nil
}

func acceptEULA() error {
	eulaPath := filepath.Join(config.ServerPath, "eula.txt")
	content := "eula=true"
	if err := os.WriteFile(eulaPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write eula.txt: %v", err)
	}
	log.Println("Minecraft EULA has been accepted automatically")
	return nil
}

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

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func startServer() error {
	if serverProcess != nil {
		return fmt.Errorf("server is already running")
	}

	jarPath := filepath.Join(config.ServerPath, "server.jar")
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		return fmt.Errorf("server jar not found at %s", jarPath)
	}

	javaArgs := []string{
		"-Xms" + config.MinRAM,
		"-Xmx" + config.MaxRAM,
		"-jar",
		"server.jar",
		"nogui",
	}

	// Start a new command window for the server
	cmd := exec.Command("cmd", "/c", "start", "Minecraft Server", "/wait", config.JavaPath)
	cmd.Dir = config.ServerPath
	cmd.Args = append(cmd.Args, javaArgs...)

	// Set up pipes for stdin, stdout, and stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %v", err)
	}
	serverStdin = stdin

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	// Start copying output to our logs
	go func() {
		scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
		for scanner.Scan() {
			log.Printf("[Server] %s", scanner.Text())
		}
	}()

	// Accept EULA before starting
	if err := acceptEULA(); err != nil {
		return fmt.Errorf("failed to accept EULA: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}

	serverProcess = cmd.Process
	log.Printf("Server started with PID %d", serverProcess.Pid)

	// Monitor the process in background
	go func() {
		state, err := cmd.Process.Wait()
		if err != nil {
			log.Printf("Server exited with error: %v", err)
		} else if !state.Success() {
			log.Printf("Server exited with code %d", state.ExitCode())
		} else {
			log.Println("Server stopped gracefully")
		}
		serverProcess = nil
		serverStdin = nil
	}()

	return nil
}

func stopServer() error {
	if serverProcess == nil {
		return nil
	}

	log.Println("Stopping server gracefully...")

	// Send 'stop' command to server stdin
	if serverStdin != nil {
		if _, err := serverStdin.Write([]byte("stop\n")); err != nil {
			return fmt.Errorf("failed to send stop command: %v", err)
		}
	}

	// Wait for the process to exit with a timeout
	done := make(chan error, 1)
	go func() {
		_, err := serverProcess.Wait()
		done <- err
	}()

	select {
	case err := <-done:
		serverProcess = nil
		serverStdin = nil
		return err
	case <-time.After(30 * time.Second):
		// Force kill if server doesn't stop gracefully
		if err := serverProcess.Kill(); err != nil {
			return fmt.Errorf("failed to force kill server: %v", err)
		}
		serverProcess = nil
		serverStdin = nil
		return fmt.Errorf("server did not stop gracefully within timeout")
	}
}

func restartServer() error {
	if err := stopServer(); err != nil {
		return fmt.Errorf("failed to stop server: %v", err)
	}

	// Wait a bit for the server to fully stop
	time.Sleep(5 * time.Second)

	if err := startServer(); err != nil {
		return fmt.Errorf("failed to restart server: %v", err)
	}

	return nil
}

func fetchJSON(url string, target interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create a buffer to store the response
	var buf bytes.Buffer
	bar := progressbar.DefaultBytes(resp.ContentLength, fmt.Sprintf("fetching json %s", url))

	// Copy the body to both the buffer and progress bar
	if _, err := io.Copy(io.MultiWriter(&buf, bar), resp.Body); err != nil {
		return err
	}

	// Add a newline after the progress bar to prevent it being mixed together
	fmt.Println()

	// Decode from the buffer
	return json.NewDecoder(&buf).Decode(target)
}

func downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	bar := progressbar.DefaultBytes(
		resp.ContentLength,
		fmt.Sprintf("downloading %s", filepath),
	)

	if _, err := io.Copy(io.MultiWriter(out, bar), resp.Body); err != nil {
		return err
	}

	// Add a newline after the progress bar to prevent it being mixed together
	fmt.Println()

	return nil
}

func saveConfig(path string) error {
	data, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
