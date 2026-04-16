// Package discover uses macOS dns-sd to find Macs advertising SSH on the LAN.
package discover

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Service represents a discovered SSH service on the LAN.
type Service struct {
	Instance string // e.g. "MAC-J7WD6G"
	Host     string // e.g. "Joshs-MacBook-Pro.local"
}

// BrowseSSH runs `dns-sd -B _ssh._tcp` for the given duration and returns unique instances.
func BrowseSSH(dur time.Duration) ([]string, error) {
	cmd := exec.Command("dns-sd", "-B", "_ssh._tcp", "local.")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	done := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		addRE := regexp.MustCompile(`\s+Add\s`)
		for scanner.Scan() {
			line := scanner.Text()
			if !addRE.MatchString(line) || !strings.Contains(line, "_ssh._tcp.") {
				continue
			}
			// Format: "HH:MM:SS.mmm  Add        2  14 local.               _ssh._tcp.           Instance Name..."
			fields := strings.Fields(line)
			if len(fields) < 7 {
				continue
			}
			instance := strings.Join(fields[6:], " ")
			seen[instance] = struct{}{}
		}
		close(done)
	}()

	time.Sleep(dur)
	_ = cmd.Process.Kill()
	<-done
	_ = cmd.Wait()

	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out, nil
}

// ResolveInstance runs `dns-sd -L <instance>` and returns the hostname.
func ResolveInstance(instance string) (string, error) {
	cmd := exec.Command("dns-sd", "-L", instance, "_ssh._tcp", "local.")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var host string
	done := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		reachedRE := regexp.MustCompile(`can be reached at (\S+):`)
		for scanner.Scan() {
			if m := reachedRE.FindStringSubmatch(scanner.Text()); m != nil {
				host = strings.TrimSuffix(m[1], ".")
				break
			}
		}
		close(done)
	}()

	time.Sleep(2 * time.Second)
	_ = cmd.Process.Kill()
	<-done
	_ = cmd.Wait()

	if host == "" {
		return "", fmt.Errorf("could not resolve %s", instance)
	}
	return host, nil
}

// DiscoverAll lists Macs on the LAN with their resolved hostnames.
func DiscoverAll(browseDur time.Duration) ([]Service, error) {
	instances, err := BrowseSSH(browseDur)
	if err != nil {
		return nil, err
	}
	out := make([]Service, 0, len(instances))
	for _, inst := range instances {
		host, _ := ResolveInstance(inst)
		if host == "" {
			host = inst
		}
		out = append(out, Service{Instance: inst, Host: host})
	}
	return out, nil
}
