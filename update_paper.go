package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func updatePaper() error {
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
	log.Printf("Fetching builds... (build number: %d, fullURL: %s)", config.BuildNumber, buildsURL)
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

	// Fetch specific build information
	buildURL := fmt.Sprintf("https://api.papermc.io/v2/projects/%s/versions/%s/builds/%d",
		projectName, currentVersion, latestBuild)
	buildInfo := &PaperAPIBuild{}
	if err := fetchJSON(buildURL, buildInfo); err != nil {
		return fmt.Errorf("failed to fetch build info: %v", err)
	}

	serverJar := filepath.Join(config.ServerPath, "server.jar")
	jarExists := true
	if _, err := os.Stat(serverJar); os.IsNotExist(err) {
		jarExists = false
	}

	needsUpdate := !jarExists || latestBuild > config.BuildNumber

	// If jar exists, verify its SHA256
	if jarExists {
		if err := verifyFileHash(serverJar, buildInfo.Downloads.Application.Sha256); err != nil {
			log.Printf("Warning: Failed to verify jar file hash: %v", err)
			needsUpdate = true
		}
	}

	if !needsUpdate {
		log.Printf("Already on latest build %d with correct checksum", config.BuildNumber)
		return nil
	}

	// Print build changes
	log.Printf("Build %d changes:", latestBuild)
	for _, change := range buildInfo.Changes {
		log.Printf("- %s", change.Summary)
	}

	if buildInfo.Channel != "default" {
		log.Printf("Warning: Using %s build %d.",
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
		return fmt.Errorf("downloaded file hash checksum mismatch: %v", err)
	}

	// Move the temporary file to the final location
	if err := os.Rename(tmpFile, serverJar); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to move downloaded file: %v", err)
	}

	// Update build number in config
	config.BuildNumber = latestBuild
	if err := saveConfig(args.Config); err != nil {
		return fmt.Errorf("failed to save config: %v", err)
	}

	log.Printf("Successfully updated to build %d", latestBuild)
	return acceptEULA()
}
