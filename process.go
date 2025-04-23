package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Variables for handling console output
var outputMutex sync.Mutex
var promptActive bool

// Helper function to print messages with proper prompt management
func printWithPrompt(message string) {
	outputMutex.Lock()
	defer outputMutex.Unlock()
	
	// If prompt is showing, clear it first
	if promptActive {
		fmt.Print("\r")  // Carriage return to beginning of line
		// Clear the line
		fmt.Print("                              \r") // Enough spaces to cover "golem> "
	}
	
	// Print the message
	fmt.Println(message)
	
	// Show the prompt again
	fmt.Print("golem> ")
	promptActive = true
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

	// Start the server process directly so pipes work correctly on all platforms
	cmd := exec.Command(config.JavaPath, javaArgs...)
	cmd.Dir = config.ServerPath

	// Set up pipes to capture server I/O
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	// Set up stdin pipe for server control
	serverStdin, err = cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %v", err)
	}

	// Accept EULA before starting
	if err := acceptEULA(); err != nil {
		return fmt.Errorf("failed to accept EULA: %v", err)
	}

	// Start the process BEFORE setting up the goroutines to read from it
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}

	serverProcess = cmd.Process
	log.Printf("Server started with PID %d", serverProcess.Pid)

	// NOW start the goroutines to capture output after the process is running
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 256*1024) // Increase buffer size
		for scanner.Scan() {
			printWithPrompt(fmt.Sprintf("[Server] %s", scanner.Text()))
		}
		if err := scanner.Err(); err != nil {
			log.Printf("Error reading server stdout: %v", err)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 4096), 256*1024) // Increase buffer size
		for scanner.Scan() {
			printWithPrompt(fmt.Sprintf("[Server Error] %s", scanner.Text()))
		}
		if err := scanner.Err(); err != nil {
			log.Printf("Error reading server stderr: %v", err)
		}
	}()

	// Start a goroutine to handle user input
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		
		// Initial welcome message with synchronized output
		outputMutex.Lock()
		fmt.Println("\nGolem is now running. Type commands to send to Minecraft server.")
		fmt.Println("Special Golem commands: !exit, !stop, !restart")
		fmt.Print("golem> ")
		promptActive = true
		outputMutex.Unlock()
		
		for scanner.Scan() {
			// Capture the input and clear prompt state
			outputMutex.Lock()
			text := scanner.Text()
			promptActive = false
			outputMutex.Unlock()
			
			// Process command
			outputMutex.Lock()
			// Handle special Golem commands
			if text == "!help" {
				fmt.Println("\nGolem Commands:")
				fmt.Println("  !help     - Display this help message")
				fmt.Println("  !exit     - Stop the server and exit Golem")
				fmt.Println("  !stop     - Stop the server but keep Golem running")
				fmt.Println("  !restart  - Restart the Minecraft server")
				fmt.Println("\nAll other commands are sent directly to the Minecraft server.")
				outputMutex.Unlock()
			} else if text == "!exit" {
				fmt.Println("Stopping server and exiting Golem...")
				outputMutex.Unlock()
				stopServer()
				os.Exit(0)
			} else if text == "!stop" {
				fmt.Println("Stopping server only...")
				outputMutex.Unlock()
				stopServer()
			} else if text == "!restart" {
				fmt.Println("Restarting server...")
				outputMutex.Unlock()
				restartServer()
			} else {
				// Send the command to the Minecraft server
				outputMutex.Unlock()
				_, err := serverStdin.Write([]byte(text + "\n"))
				if err != nil {
					printWithPrompt(fmt.Sprintf("Error sending command to server: %v", err))
					// Continue so we still get the prompt back
					outputMutex.Lock()
				}
			}
			
			// Reshow prompt if needed
			if !promptActive {
				outputMutex.Lock()
				fmt.Print("golem> ")
				promptActive = true
				outputMutex.Unlock()
			}
		}
		
		if err := scanner.Err(); err != nil {
			log.Printf("Error reading user input: %v", err)
		}
	}()
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
