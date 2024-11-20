package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

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

func updateServer() error {
	// Create server directory if it doesn't exist
	if err := os.MkdirAll(config.ServerPath, 0755); err != nil {
		return fmt.Errorf("failed to create server directory: %v", err)
	}

	// Update server based on type
	switch config.ServerType {
	case Paper:
		return updatePaper()
	case Purpur:
		return updatePurpur()
	case Vanilla:
		return fmt.Errorf("vanilla server type not implemented yet")
	default:
		return fmt.Errorf("unknown server type: %s", config.ServerType)
	}
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
