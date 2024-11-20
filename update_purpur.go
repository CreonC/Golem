package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type PurpurAPIResponse struct {
	Project   string `json:"project"`
	Version   string `json:"version"`
	Build     string `json:"build"`
	Result    string `json:"result"`
	Timestamp int64  `json:"timestamp"`
	Duration  int    `json:"duration"`
	Commits   []struct {
		// Add commit fields if needed
	} `json:"commits"`
	Metadata struct {
		Type string `json:"type"`
	} `json:"metadata"`
	Md5 string `json:"md5"`
}

func updatePurpur() error {
	projectName := strings.ToLower(string(config.ServerType))
	currentVersion := config.ServerVersion

	// Get latest build info
	buildURL := fmt.Sprintf("https://api.purpurmc.org/v2/%s/%s/%d",
		projectName, currentVersion, config.BuildNumber)
	log.Printf("Fetching build info from: %s", buildURL)

	buildInfo := &PurpurAPIResponse{}
	if err := fetchJSON(buildURL, buildInfo); err != nil {
		return fmt.Errorf("failed to fetch build info: %v", err)
	}

	serverJar := filepath.Join(config.ServerPath, "server.jar")
	jarExists := true
	if _, err := os.Stat(serverJar); os.IsNotExist(err) {
		jarExists = false
	}

	latestBuildNum := buildInfo.Build
	needsUpdate := !jarExists || config.BuildNumber < parseInt(latestBuildNum)

	// If jar exists, verify its MD5
	if jarExists {
		matches, err := verifyMD5(serverJar, buildInfo.Md5)
		if err != nil {
			log.Printf("Warning: Failed to verify jar file MD5: %v", err)
			needsUpdate = true
		} else if !matches {
			log.Printf("MD5 checksum mismatch, will redownload jar file")
			needsUpdate = true
		}
	}

	if needsUpdate {
		// Download new version
		downloadURL := fmt.Sprintf("https://api.purpurmc.org/v2/%s/%s/%s/download",
			projectName, currentVersion, latestBuildNum)

		log.Printf("Downloading new build %s", latestBuildNum)

		// Create a temporary file for downloading
		tmpFile := serverJar + ".tmp"
		if err := downloadFile(downloadURL, tmpFile); err != nil {
			os.Remove(tmpFile)
			return fmt.Errorf("failed to download new server jar: %v", err)
		}

		// Verify the downloaded file
		matches, err := verifyMD5(tmpFile, buildInfo.Md5)
		if err != nil {
			os.Remove(tmpFile)
			return fmt.Errorf("failed to verify downloaded file: %v", err)
		}
		if !matches {
			os.Remove(tmpFile)
			return fmt.Errorf("downloaded file MD5 checksum mismatch")
		}

		// Move the temporary file to the final location
		if err := os.Rename(tmpFile, serverJar); err != nil {
			os.Remove(tmpFile)
			return fmt.Errorf("failed to move downloaded file: %v", err)
		}

		// Update build number in config
		config.BuildNumber = parseInt(latestBuildNum)
		if err := saveConfig(args.Config); err != nil {
			return fmt.Errorf("failed to save config: %v", err)
		}

		log.Printf("Successfully updated to build %s", latestBuildNum)
	} else {
		log.Printf("Already on latest build %d with correct checksum", config.BuildNumber)
	}

	return acceptEULA()
}

func parseInt(s string) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}
