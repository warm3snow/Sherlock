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

package sshclient

import (
	"errors"

	"github.com/pkg/sftp"
)

// SFTPClient wraps an SFTP client with the underlying SSH client.
type SFTPClient struct {
	sshClient *Client
	sftp      *sftp.Client
}

// NewSFTPSession creates a new SFTP session from the existing SSH connection.
func (c *Client) NewSFTPSession() (*SFTPClient, error) {
	if !c.isConnected {
		return nil, errors.New("not connected")
	}

	sftpClient, err := sftp.NewClient(c.client)
	if err != nil {
		return nil, err
	}

	return &SFTPClient{
		sshClient: c,
		sftp:      sftpClient,
	}, nil
}

// SFTP returns the underlying sftp.Client for direct operations.
func (s *SFTPClient) SFTP() *sftp.Client {
	return s.sftp
}

// Close closes the SFTP session.
func (s *SFTPClient) Close() error {
	if s.sftp != nil {
		return s.sftp.Close()
	}
	return nil
}
