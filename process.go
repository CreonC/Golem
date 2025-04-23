package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chzyer/readline"
)

// Variables for handling console output
var outputMutex sync.Mutex
var promptActive bool
var instance *readline.Instance
var processDown chan struct{} // Channel to signal when the server process has exited

// No problematic server-side tab completion

// GolemCompleter implements the readline.AutoCompleter interface for tab completion
type GolemCompleter struct{}

// Do implements tab completion for Golem commands and basic Minecraft commands
func (c *GolemCompleter) Do(line []rune, pos int) ([][]rune, int) {
	lineStr := string(line[:pos])
	prefix := lineStr
	
	// Golem commands start with !
	if strings.HasPrefix(lineStr, "!") {
		// List of all Golem commands
		commands := []string{
			"!help", "!exit", "!stop", "!restart",
		}
		
		matched := [][]rune{}
		for _, cmd := range commands {
			if strings.HasPrefix(cmd, lineStr) {
				matched = append(matched, []rune(strings.TrimPrefix(cmd, lineStr)))
			}
		}
		
		return matched, len(prefix)
	}
	
	// For Minecraft commands, use a client-side list of common commands
	// List of common Minecraft server commands
	minecraftCommands := []string{
		"ban", "ban-ip", "banlist", "clear", "deop", "difficulty", "effect", 
		"enchant", "gamemode", "gamerule", "give", "help", "kick", "kill", 
		"list", "me", "op", "pardon", "pardon-ip", "plugins", "reload", 
		"save-all", "save-off", "save-on", "say", "scoreboard", "seed", 
		"setidletimeout", "setworldspawn", "spawnpoint", "stop", "tell", 
		"teleport", "time", "timings", "tp", "version", "weather", "whitelist",
		"world", "xp",
	}
	
	matched := [][]rune{}
	for _, cmd := range minecraftCommands {
		if strings.HasPrefix(cmd, lineStr) {
			matched = append(matched, []rune(strings.TrimPrefix(cmd, lineStr)))
		}
	}
	
	return matched, len(prefix)
}

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

	// Initialize the processDown channel
	processDown = make(chan struct{})
	
	// Monitor the process in background first to signal other goroutines
	go func() {
		state, err := cmd.Process.Wait()
		if err != nil {
			log.Printf("Server exited with error: %v", err)
		} else if !state.Success() {
			log.Printf("Server exited with code %d", state.ExitCode())
		} else {
			log.Println("Server stopped gracefully")
		}
		
		// Signal other goroutines to stop
		close(processDown)
		
		// Clean up process variables
		serverProcess = nil
		serverStdin = nil
	}()
	
	// NOW start the goroutines to capture output after the process is running
	go func() {
		defer func() {
			// Recover from any panics that might occur if pipes are closed
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in stdout reader: %v", r)
			}
		}()
		
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 256*1024) // Increase buffer size
		
		for {
			select {
			case <-processDown:
				// Process is down, exit goroutine
				return
			default:
				// Read next line if available
				if !scanner.Scan() {
					// End of output or error
					if err := scanner.Err(); err != nil && err != io.EOF {
						log.Printf("Error reading server stdout: %v", err)
					}
					return
				}
				printWithPrompt(fmt.Sprintf("[Server] %s", scanner.Text()))
			}
		}
	}()
	
	go func() {
		defer func() {
			// Recover from any panics that might occur if pipes are closed
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in stderr reader: %v", r)
			}
		}()
		
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 4096), 256*1024) // Increase buffer size
		
		for {
			select {
			case <-processDown:
				// Process is down, exit goroutine
				return
			default:
				// Read next line if available
				if !scanner.Scan() {
					// End of output or error
					if err := scanner.Err(); err != nil && err != io.EOF {
						log.Printf("Error reading server stderr: %v", err)
					}
					return
				}
				printWithPrompt(fmt.Sprintf("[Server Error] %s", scanner.Text()))
			}
		}
	}()
	
	// Setup readline for tab completion and better input handling
	var rlErr error
	instance, rlErr = readline.NewEx(&readline.Config{
		Prompt:       "golem> ",
		AutoComplete: &GolemCompleter{},
		// Prevent readline from directly exiting on Ctrl-C
		InterruptPrompt: "^C",
		EOFPrompt:      "exit",
	})
	if rlErr != nil {
		return fmt.Errorf("failed to initialize readline: %v", rlErr)
	}
	
	// Start a goroutine to handle user input with readline
	go func() {
		// Initial welcome message
		fmt.Println("\nGolem is now running. Type commands to send to Minecraft server.")
		fmt.Println("Special Golem commands: !help, !exit, !stop, !restart")
		fmt.Println("Use TAB for command completion")
		
		defer instance.Close()
		
		for {
			// Read a line with tab completion
			line, err := instance.Readline()
			if err != nil {
				// Handle readline specific errors
				if err == readline.ErrInterrupt {
					// Ctrl-C pressed, but don't exit
					continue
				} else if err == io.EOF {
					// EOF (Ctrl-D) - exit gracefully
					fmt.Println("Exiting Golem...")
					if serverProcess != nil {
						stopServer()
					}
					os.Exit(0)
				} else {
					// Other error
					log.Printf("Error reading input: %v", err)
					break
				}
			}
			
			// Process command
			// Handle special Golem commands
			switch line {
			case "!help":
				fmt.Println("\nGolem Commands:")
				fmt.Println("  !help     - Display this help message")
				fmt.Println("  !exit     - Stop the server and exit Golem")
				fmt.Println("  !stop     - Stop the server but keep Golem running")
				fmt.Println("  !restart  - Restart the Minecraft server")
				fmt.Println("\nAll other commands are sent directly to the Minecraft server.")
				
			case "!exit":
				fmt.Println("Stopping server and exiting Golem...")
				stopServer()
				os.Exit(0)
				
			case "!stop":
				fmt.Println("Stopping server only...")
				stopServer()
				
			case "!restart":
				fmt.Println("Restarting server...")
				restartServer()
				
			default:
				// Check if server is still running before sending commands
				if serverProcess == nil || serverStdin == nil {
					fmt.Println("Server is not running. Use !restart to start it again.")
					continue
				}
				
				// Send the command to the Minecraft server
				_, err := serverStdin.Write([]byte(line + "\n"))
				if err != nil {
					fmt.Printf("Error sending command to server: %v\n", err)
				}
			}
		}
		
		log.Println("Input handler exited")
	}()
// Process monitoring has been moved to the top of this function
	// No need to monitor again here

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
	// Only try to stop if there's a server process running
	if serverProcess != nil {
		log.Println("Stopping server before restart...")
		if err := stopServer(); err != nil {
			// If it failed because there's no process, that's fine
			if !strings.Contains(err.Error(), "no child processes") {
				log.Printf("Warning during server stop: %v", err)
			}
		}
		// Make sure these are nil to avoid issues
		serverProcess = nil
		serverStdin = nil
	} else {
		log.Println("No server process found to stop, starting fresh")
	}

	// Wait a bit for the server to fully stop and resources to be released
	log.Println("Waiting 3 seconds before restart...")
	time.Sleep(3 * time.Second)

	// Start the server
	log.Println("Starting server...")
	if err := startServer(); err != nil {
		return fmt.Errorf("failed to restart server: %v", err)
	}

	return nil
}
