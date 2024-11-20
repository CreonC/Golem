package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

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
	// FIXME: Windows only, will not work on other platforms
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
