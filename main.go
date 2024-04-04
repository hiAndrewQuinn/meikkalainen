package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// HostConfig  holds the configuration for a host.
type HostConfig struct {
	Hostname       string
	PrivateKeyPath string
	Port           int
}

type DebianSystemDetails struct {
	Timestamp        time.Time     `json:"timestamp"`
	DebianVersion    string        `json:"debian_version"`
	Architecture     string        `json:"architecture"`
	KernelVersion    string        `json:"kernel_version"`
	InstalledModules []string      `json:"installed_modules"`
	NetworkConfig    NetworkConfig `json:"network_config"`
	// Plus any existing fields...
	SystemdUnits []SystemdUnit  `json:"units"`     // Adjust this field name if different
	Libraries    []InstalledLib `json:"libraries"` // Adjust this field name if different
}

type NetworkConfig struct {
	IPAddresses []string `json:"ip_addresses"`
	Interfaces  []string `json:"interfaces"`
	RoutingInfo string   `json:"routing_info"`
}

type SystemdUnit struct {
	Name        string `json:"name"`
	LoadState   string `json:"load_state"`
	ActiveState string `json:"active_state"`
	Description string `json:"description"`
}

// InstalledLib represents a library installed on the system.
type InstalledLib struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: meikkalainen [user@hostname --private-key path/to/key]...")
		os.Exit(1)
	}

	hostConfigs, err := parseHostConfigs(os.Args[1:])
	if err != nil {
		log.Fatalf("Error parsing arguments: %v", err)
	}

	for _, config := range hostConfigs {
		handleHost(config) // Pass the whole config to handleHost
	}
}

func parseHostConfigs(args []string) ([]HostConfig, error) {
	var hostConfigs []HostConfig
	defaultPrivateKeyPath := "/default/path/to/private/key"
	defaultPort := 22 // Default SSH port

	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			// Skip flags here as they will be processed in the next iterations
			continue
		}

		config := HostConfig{Hostname: args[i], PrivateKeyPath: defaultPrivateKeyPath, Port: defaultPort}

		// Look ahead for flags related to this host
		for i+1 < len(args) && strings.HasPrefix(args[i+1], "--") {
			switch args[i+1] {
			case "--private-key":
				if i+2 < len(args) {
					config.PrivateKeyPath = args[i+2]
					i += 2
				} else {
					return nil, fmt.Errorf("--private-key flag without a value")
				}
			case "--port":
				if i+2 < len(args) {
					port, err := strconv.Atoi(args[i+2])
					if err != nil {
						return nil, fmt.Errorf("--port flag with invalid value %s", args[i+2])
					}
					config.Port = port
					i += 2
				} else {
					return nil, fmt.Errorf("--port flag without a value")
				}
			}
		}

		hostConfigs = append(hostConfigs, config)
	}

	return hostConfigs, nil
}

func handleHost(config HostConfig) {
	fmt.Printf("Handling host: %s with private key: %s\n", config.Hostname, config.PrivateKeyPath)

	// Split the host identifier into username and hostname.
	parts := strings.SplitN(config.Hostname, "@", 2)
	if len(parts) != 2 {
		fmt.Println("Invalid host format. Expected user@hostname.")
		return
	}
	user, hostname := parts[0], parts[1]

	// Set up the SSH client configuration using the provided private key.
	sshConfig, err := sshClientConfig(user, config.PrivateKeyPath)
	if err != nil {
		fmt.Printf("Failed to set up SSH config for host %s: %v\n", config.Hostname, err)
		return
	}

	// Format the address with the port
	address := fmt.Sprintf("%s:%d", hostname, config.Port)

	// Connect to the SSH server using the address with the specified port
	client, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		fmt.Printf("Failed to dial SSH for host %s: %v\n", config.Hostname, err)
		return
	}
	defer client.Close()

	// Use the client to fetch data.
	details, err := fetchData(client)
	if err != nil {
		fmt.Printf("Failed to fetch data for host %s: %v\n", config.Hostname, err)
		return
	}

	// Serialize details to JSON and save.
	if err := saveDetailsAsJSON(hostname, user, details); err != nil {
		fmt.Printf("Failed to save data for host %s: %v\n", config.Hostname, err)
	}
}

func fetchData(client *ssh.Client) (*DebianSystemDetails, error) {
	// Initialize the details structure with the current timestamp.
	details := DebianSystemDetails{
		Timestamp: time.Now(),
	}

	var err error
	// Fetch Debian version.
	details.DebianVersion, err = executeCommand(client, "cat /etc/debian_version")
	if err != nil {
		return nil, fmt.Errorf("error fetching Debian version: %w", err)
	}

	// Fetch architecture
	architecture, err := executeCommand(client, "uname -m")
	if err != nil {
		return nil, fmt.Errorf("error fetching architecture: %w", err)
	}
	details.Architecture = strings.TrimSpace(architecture)

	// Fetch kernel version
	kernelVersion, err := executeCommand(client, "uname -r")
	if err != nil {
		return nil, fmt.Errorf("error fetching kernel version: %w", err)
	}
	details.KernelVersion = strings.TrimSpace(kernelVersion)

	// Fetch installed kernel modules
	installedModulesOutput, err := executeCommand(client, "lsmod")
	if err != nil {
		return nil, fmt.Errorf("error fetching installed kernel modules: %w", err)
	}
	details.InstalledModules = parseLsmodOutput(installedModulesOutput)
	sort.Strings(details.InstalledModules)

	// Fetch network configuration
	ipAddressesOutput, err := executeCommand(client, "hostname -I")
	if err != nil {
		return nil, fmt.Errorf("error fetching IP addresses: %w", err)
	}
	details.NetworkConfig.IPAddresses = strings.Fields(strings.TrimSpace(ipAddressesOutput))

	interfacesOutput, err := executeCommand(client, "ls /sys/class/net")
	if err != nil {
		return nil, fmt.Errorf("error fetching network interfaces: %w", err)
	}
	details.NetworkConfig.Interfaces = strings.Fields(strings.TrimSpace(interfacesOutput))

	routingInfoOutput, err := executeCommand(client, "ip route")
	if err != nil {
		return nil, fmt.Errorf("error fetching routing information: %w", err)
	}
	details.NetworkConfig.RoutingInfo = strings.TrimSpace(routingInfoOutput)

	// Fetch systemd unit states.
	unitOutput, err := executeCommand(client, "systemctl list-units --output=export | tail -n +2 | sort")
	if err != nil {
		return nil, fmt.Errorf("error fetching systemd unit states: %w", err)
	}
	details.SystemdUnits = parseSystemdOutput(unitOutput)

	// Fetch installed libraries with dpkg.
	libOutput, err := executeCommand(client, "dpkg-query --show")
	if err != nil {
		return nil, fmt.Errorf("error fetching installed libraries: %w", err)
	}
	details.Libraries = parseDpkgOutput(libOutput)

	return &details, nil
}

func saveDetailsAsJSON(hostname, user string, details *DebianSystemDetails) error {
	jsonData, err := json.MarshalIndent(details, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to serialize data: %w", err)
	}

	filename := fmt.Sprintf("json/%s/%s_%s.json", hostname, user, details.Timestamp.Format("2006_01_02_15_04_05"))
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	if err := ioutil.WriteFile(filename, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	fmt.Printf("Successfully saved data to %s\n", filename)
	return nil
}

func sshClientConfig(user, privateKeyPath string) (*ssh.ClientConfig, error) {
	key, err := ioutil.ReadFile(privateKeyPath)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Matches StrictHostKeyChecking=no and UserKnownHostsFile=/dev/null
		BannerCallback:  ssh.BannerDisplayStderr(),
		ClientVersion:   "SSH-2.0-OpenSSH_7.9", // Example, adjust as needed
		Timeout:         0,                     // Consider setting a timeout
	}

	// Note: Go's SSH package doesn't expose options equivalent to IdentitiesOnly, LogLevel, PubkeyAcceptedKeyTypes, and HostKeyAlgorithms directly.
	// Some of these settings relate to security policies and logging, which are handled differently in Go applications.

	return config, nil
}

// executeCommand executes a shell command on the remote system using the provided ssh.Client and returns the output.
func executeCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdoutBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	err = session.Run(command)
	if err != nil {
		return "", fmt.Errorf("failed to run command '%s': %w", command, err)
	}

	return stdoutBuf.String(), nil
}

func parseLsmodOutput(output string) []string {
	modules := make([]string, 0)
	lines := strings.Split(output, "\n")
	for _, line := range lines[1:] { // Skip header line
		fields := strings.Fields(line)
		if len(fields) > 0 {
			modules = append(modules, fields[0])
		}
	}
	return modules
}

// parseSystemdOutput parses the output from the systemd list-units command and returns unit states.
func parseSystemdOutput(output string) []SystemdUnit {
	var units []SystemdUnit
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			// Concatenate the description back together if it was split.
			description := strings.Join(fields[4:], " ")

			unit := SystemdUnit{
				Name:        fields[0],
				LoadState:   fields[1],
				ActiveState: fields[2],
				Description: description,
			}
			units = append(units, unit)
		}
	}

	return units
}

// parseDpkgOutput parses the output from `dpkg-query --show` command and returns installed libraries.
func parseDpkgOutput(output string) []InstalledLib {
	var installedLibs []InstalledLib

	// Split the output into lines.
	lines := strings.Split(output, "\n")

	// Iterate through each line to extract package information.
	for _, line := range lines {
		// Skip empty lines
		if line == "" {
			continue
		}

		// Split each line into package name and version based on the tab separator.
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 {
			continue // Skip lines that do not conform to expected format.
		}

		// Construct an InstalledLib object with the parsed name and version.
		installedLib := InstalledLib{
			Name:    fields[0],
			Version: fields[1],
		}

		// Append the constructed object to the slice of installed libraries.
		installedLibs = append(installedLibs, installedLib)
	}

	return installedLibs
}
