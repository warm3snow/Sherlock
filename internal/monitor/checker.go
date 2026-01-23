// Copyright 2024 Sherlock Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package monitor provides host monitoring and alerting functionality.
package monitor

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/warm3snow/sherlock/internal/hosts"
	"github.com/warm3snow/sherlock/pkg/sshclient"
)

// HostStatus represents the status of a host.
type HostStatus struct {
	Host         *hosts.Host
	Reachable    bool
	SSHAvailable bool
	Latency      time.Duration
	LastChecked  time.Time
	Error        error
}

// Checker checks host connectivity and status.
type Checker struct {
	hostManager    *hosts.Manager
	privateKeyPath string
	timeout        time.Duration
}

// NewChecker creates a new host checker.
func NewChecker(hostManager *hosts.Manager, privateKeyPath string) *Checker {
	return &Checker{
		hostManager:    hostManager,
		privateKeyPath: privateKeyPath,
		timeout:        5 * time.Second,
	}
}

// SetTimeout sets the check timeout.
func (c *Checker) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

// CheckHost checks a single host's connectivity.
func (c *Checker) CheckHost(ctx context.Context, host *hosts.Host) *HostStatus {
	status := &HostStatus{
		Host:        host,
		LastChecked: time.Now(),
	}

	// Check TCP connectivity first
	startTime := time.Now()
	addr := fmt.Sprintf("%s:%d", host.Host, host.Port)

	conn, err := net.DialTimeout("tcp", addr, c.timeout)
	if err != nil {
		status.Reachable = false
		status.Error = fmt.Errorf("connection failed: %w", err)
		return status
	}
	conn.Close()

	status.Reachable = true
	status.Latency = time.Since(startTime)

	// Try SSH authentication
	cfg := &sshclient.Config{
		HostInfo: &sshclient.HostInfo{
			Host: host.Host,
			Port: host.Port,
			User: host.User,
		},
		PrivateKeyPath: c.privateKeyPath,
		Timeout:        c.timeout,
	}

	client, err := sshclient.NewClient(cfg)
	if err != nil {
		status.SSHAvailable = false
		status.Error = fmt.Errorf("SSH client creation failed: %w", err)
		return status
	}
	defer client.Close()

	if err := client.Connect(ctx); err != nil {
		status.SSHAvailable = false
		status.Error = fmt.Errorf("SSH connection failed: %w", err)
		return status
	}

	status.SSHAvailable = true
	return status
}

// CheckHosts checks multiple hosts concurrently.
func (c *Checker) CheckHosts(ctx context.Context, hostList []hosts.Host, concurrency int) []HostStatus {
	if concurrency <= 0 {
		concurrency = 10
	}

	results := make([]HostStatus, 0, len(hostList))
	resultChan := make(chan HostStatus, len(hostList))
	semaphore := make(chan struct{}, concurrency)

	var wg sync.WaitGroup
	for _, host := range hostList {
		wg.Add(1)
		go func(h hosts.Host) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			status := c.CheckHost(ctx, &h)
			resultChan <- *status
		}(host)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for status := range resultChan {
		results = append(results, status)
	}

	return results
}

// CheckAllHosts checks all hosts in the database.
func (c *Checker) CheckAllHosts(ctx context.Context, concurrency int) ([]HostStatus, error) {
	hostList, err := c.hostManager.GetAllHosts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get hosts: %w", err)
	}

	return c.CheckHosts(ctx, hostList, concurrency), nil
}

// CheckGroup checks all hosts in a group.
func (c *Checker) CheckGroup(ctx context.Context, groupName string, concurrency int) ([]HostStatus, error) {
	hostList, err := c.hostManager.GetGroupHostsByName(ctx, groupName)
	if err != nil {
		return nil, fmt.Errorf("failed to get group hosts: %w", err)
	}

	return c.CheckHosts(ctx, hostList, concurrency), nil
}

// FormatStatusTable formats host statuses as a table.
func FormatStatusTable(statuses []HostStatus) string {
	var result string
	result += "\nHost Status:\n"
	result += "--------------------------------------------------------------------------------\n"
	result += fmt.Sprintf("%-30s | %-8s | %-10s | %-8s | %s\n", "Host", "Status", "Latency", "SSH", "Error")
	result += "--------------------------------------------------------------------------------\n"

	for _, s := range statuses {
		hostName := s.Host.DisplayName()
		if len(hostName) > 30 {
			hostName = hostName[:27] + "..."
		}

		status := "DOWN"
		if s.Reachable {
			status = "UP"
		}

		latency := "-"
		if s.Latency > 0 {
			latency = fmt.Sprintf("%dms", s.Latency.Milliseconds())
		}

		ssh := "-"
		if s.Reachable {
			if s.SSHAvailable {
				ssh = "OK"
			} else {
				ssh = "FAIL"
			}
		}

		errMsg := ""
		if s.Error != nil {
			errMsg = s.Error.Error()
			if len(errMsg) > 30 {
				errMsg = errMsg[:27] + "..."
			}
		}

		result += fmt.Sprintf("%-30s | %-8s | %-10s | %-8s | %s\n", hostName, status, latency, ssh, errMsg)
	}

	result += "--------------------------------------------------------------------------------\n"

	// Summary
	up := 0
	down := 0
	sshOK := 0
	for _, s := range statuses {
		if s.Reachable {
			up++
			if s.SSHAvailable {
				sshOK++
			}
		} else {
			down++
		}
	}
	result += fmt.Sprintf("Summary: %d up, %d down, %d SSH OK\n", up, down, sshOK)

	return result
}
