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

// Package transfer provides file transfer functionality over SSH.
package transfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"github.com/warm3snow/sherlock/pkg/sshclient"
)

// TransferProgress represents file transfer progress.
type TransferProgress struct {
	Filename         string
	TotalBytes       int64
	TransferredBytes int64
	Percentage       float64
	Speed            float64 // bytes per second
	ETA              time.Duration
}

// ProgressCallback is called during transfer with progress updates.
type ProgressCallback func(progress *TransferProgress)

// Options configures file transfer.
type Options struct {
	Recursive      bool             // Transfer directories recursively
	Resume         bool             // Resume interrupted transfers (not implemented yet)
	PreserveMode   bool             // Preserve file permissions
	PreserveTime   bool             // Preserve timestamps
	Overwrite      bool             // Overwrite existing files
	BufferSize     int              // Buffer size for transfer
	OnProgress     ProgressCallback // Progress callback
}

// DefaultOptions returns default transfer options.
func DefaultOptions() *Options {
	return &Options{
		Recursive:    false,
		Resume:       false,
		PreserveMode: true,
		PreserveTime: true,
		Overwrite:    true,
		BufferSize:   32 * 1024, // 32KB
	}
}

// Result represents the result of a transfer operation.
type Result struct {
	Source           string
	Destination      string
	BytesTransferred int64
	FileCount        int
	DirCount         int
	Duration         time.Duration
	Error            error
}

// FileTransfer handles file transfer operations.
type FileTransfer struct {
	client     *sshclient.Client
	sftpClient *sshclient.SFTPClient
	sftp       *sftp.Client
}

// NewFileTransfer creates a new file transfer handler.
func NewFileTransfer(client *sshclient.Client) (*FileTransfer, error) {
	sftpClient, err := client.NewSFTPSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SFTP session: %w", err)
	}

	return &FileTransfer{
		client:     client,
		sftpClient: sftpClient,
		sftp:       sftpClient.SFTP(),
	}, nil
}

// Close closes the file transfer handler.
func (t *FileTransfer) Close() error {
	if t.sftpClient != nil {
		return t.sftpClient.Close()
	}
	return nil
}

// Upload transfers local files to remote host.
func (t *FileTransfer) Upload(ctx context.Context, localPath, remotePath string, opts *Options) (*Result, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	startTime := time.Now()
	result := &Result{
		Source:      localPath,
		Destination: remotePath,
	}

	// Check if local path exists
	localInfo, err := os.Stat(localPath)
	if err != nil {
		result.Error = fmt.Errorf("local path not found: %w", err)
		result.Duration = time.Since(startTime)
		return result, result.Error
	}

	// If remotePath is a directory (ends with / or is an existing directory), append local filename
	if !localInfo.IsDir() {
		if strings.HasSuffix(remotePath, "/") {
			// User explicitly specified a directory
			remotePath = filepath.Join(remotePath, filepath.Base(localPath))
		} else {
			// Check if remote path is an existing directory
			if remoteInfo, err := t.sftp.Stat(remotePath); err == nil && remoteInfo.IsDir() {
				remotePath = filepath.Join(remotePath, filepath.Base(localPath))
			}
		}
		result.Destination = remotePath
	}

	if localInfo.IsDir() {
		if !opts.Recursive {
			result.Error = errors.New("cannot upload directory without recursive option")
			result.Duration = time.Since(startTime)
			return result, result.Error
		}
		err = t.uploadDirectory(ctx, localPath, remotePath, opts, result)
	} else {
		err = t.uploadFile(ctx, localPath, remotePath, localInfo, opts, result)
	}

	result.Error = err
	result.Duration = time.Since(startTime)
	return result, err
}

// Download transfers remote files to local host.
func (t *FileTransfer) Download(ctx context.Context, remotePath, localPath string, opts *Options) (*Result, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	startTime := time.Now()
	result := &Result{
		Source:      remotePath,
		Destination: localPath,
	}

	// Check if remote path exists
	remoteInfo, err := t.sftp.Stat(remotePath)
	if err != nil {
		result.Error = fmt.Errorf("remote path not found: %w", err)
		result.Duration = time.Since(startTime)
		return result, result.Error
	}

	// If localPath is a directory (ends with / or is an existing directory), append remote filename
	if !remoteInfo.IsDir() {
		if strings.HasSuffix(localPath, "/") || strings.HasSuffix(localPath, string(os.PathSeparator)) {
			// User explicitly specified a directory
			localPath = filepath.Join(localPath, filepath.Base(remotePath))
		} else {
			// Check if local path is an existing directory
			if localInfo, err := os.Stat(localPath); err == nil && localInfo.IsDir() {
				localPath = filepath.Join(localPath, filepath.Base(remotePath))
			}
		}
		result.Destination = localPath
	}

	if remoteInfo.IsDir() {
		if !opts.Recursive {
			result.Error = errors.New("cannot download directory without recursive option")
			result.Duration = time.Since(startTime)
			return result, result.Error
		}
		err = t.downloadDirectory(ctx, remotePath, localPath, opts, result)
	} else {
		err = t.downloadFile(ctx, remotePath, localPath, remoteInfo, opts, result)
	}

	result.Error = err
	result.Duration = time.Since(startTime)
	return result, err
}

// uploadFile uploads a single file.
func (t *FileTransfer) uploadFile(ctx context.Context, localPath, remotePath string, localInfo os.FileInfo, opts *Options, result *Result) error {
	// Check context
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Open local file
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	// Ensure remote directory exists
	remoteDir := filepath.Dir(remotePath)
	if remoteDir != "" && remoteDir != "." {
		if err := t.sftp.MkdirAll(remoteDir); err != nil {
			// Ignore if directory already exists
		}
	}

	// Check if remote file exists
	if !opts.Overwrite {
		if _, err := t.sftp.Stat(remotePath); err == nil {
			return fmt.Errorf("remote file already exists: %s", remotePath)
		}
	}

	// Create remote file
	remoteFile, err := t.sftp.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %w", err)
	}
	defer remoteFile.Close()

	// Copy with progress
	fileSize := localInfo.Size()
	buffer := make([]byte, opts.BufferSize)
	var transferred int64
	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := localFile.Read(buffer)
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read local file: %w", err)
		}
		if n == 0 {
			break
		}

		written, err := remoteFile.Write(buffer[:n])
		if err != nil {
			return fmt.Errorf("failed to write remote file: %w", err)
		}

		transferred += int64(written)
		result.BytesTransferred += int64(written)

		// Report progress
		if opts.OnProgress != nil {
			elapsed := time.Since(startTime).Seconds()
			speed := float64(transferred) / elapsed
			var eta time.Duration
			if speed > 0 {
				eta = time.Duration(float64(fileSize-transferred)/speed) * time.Second
			}

			opts.OnProgress(&TransferProgress{
				Filename:         filepath.Base(localPath),
				TotalBytes:       fileSize,
				TransferredBytes: transferred,
				Percentage:       float64(transferred) / float64(fileSize) * 100,
				Speed:            speed,
				ETA:              eta,
			})
		}
	}

	// Preserve permissions
	if opts.PreserveMode {
		if err := t.sftp.Chmod(remotePath, localInfo.Mode()); err != nil {
			// Log but don't fail
		}
	}

	// Preserve timestamps
	if opts.PreserveTime {
		if err := t.sftp.Chtimes(remotePath, localInfo.ModTime(), localInfo.ModTime()); err != nil {
			// Log but don't fail
		}
	}

	result.FileCount++
	return nil
}

// uploadDirectory uploads a directory recursively.
func (t *FileTransfer) uploadDirectory(ctx context.Context, localPath, remotePath string, opts *Options, result *Result) error {
	// Create remote directory
	if err := t.sftp.MkdirAll(remotePath); err != nil {
		return fmt.Errorf("failed to create remote directory: %w", err)
	}
	result.DirCount++

	entries, err := os.ReadDir(localPath)
	if err != nil {
		return fmt.Errorf("failed to read local directory: %w", err)
	}

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		localEntryPath := filepath.Join(localPath, entry.Name())
		remoteEntryPath := filepath.Join(remotePath, entry.Name())

		if entry.IsDir() {
			if err := t.uploadDirectory(ctx, localEntryPath, remoteEntryPath, opts, result); err != nil {
				return err
			}
		} else {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if err := t.uploadFile(ctx, localEntryPath, remoteEntryPath, info, opts, result); err != nil {
				return err
			}
		}
	}

	return nil
}

// downloadFile downloads a single file.
func (t *FileTransfer) downloadFile(ctx context.Context, remotePath, localPath string, remoteInfo os.FileInfo, opts *Options, result *Result) error {
	// Check context
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Open remote file
	remoteFile, err := t.sftp.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file: %w", err)
	}
	defer remoteFile.Close()

	// Ensure local directory exists
	localDir := filepath.Dir(localPath)
	if localDir != "" && localDir != "." {
		if err := os.MkdirAll(localDir, 0755); err != nil {
			return fmt.Errorf("failed to create local directory: %w", err)
		}
	}

	// Check if local file exists
	if !opts.Overwrite {
		if _, err := os.Stat(localPath); err == nil {
			return fmt.Errorf("local file already exists: %s", localPath)
		}
	}

	// Create local file
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFile.Close()

	// Copy with progress
	fileSize := remoteInfo.Size()
	buffer := make([]byte, opts.BufferSize)
	var transferred int64
	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := remoteFile.Read(buffer)
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read remote file: %w", err)
		}
		if n == 0 {
			break
		}

		written, err := localFile.Write(buffer[:n])
		if err != nil {
			return fmt.Errorf("failed to write local file: %w", err)
		}

		transferred += int64(written)
		result.BytesTransferred += int64(written)

		// Report progress
		if opts.OnProgress != nil {
			elapsed := time.Since(startTime).Seconds()
			speed := float64(transferred) / elapsed
			var eta time.Duration
			if speed > 0 {
				eta = time.Duration(float64(fileSize-transferred)/speed) * time.Second
			}

			opts.OnProgress(&TransferProgress{
				Filename:         filepath.Base(remotePath),
				TotalBytes:       fileSize,
				TransferredBytes: transferred,
				Percentage:       float64(transferred) / float64(fileSize) * 100,
				Speed:            speed,
				ETA:              eta,
			})
		}
	}

	// Preserve permissions
	if opts.PreserveMode {
		if err := os.Chmod(localPath, remoteInfo.Mode()); err != nil {
			// Log but don't fail
		}
	}

	// Preserve timestamps
	if opts.PreserveTime {
		if err := os.Chtimes(localPath, remoteInfo.ModTime(), remoteInfo.ModTime()); err != nil {
			// Log but don't fail
		}
	}

	result.FileCount++
	return nil
}

// downloadDirectory downloads a directory recursively.
func (t *FileTransfer) downloadDirectory(ctx context.Context, remotePath, localPath string, opts *Options, result *Result) error {
	// Create local directory
	if err := os.MkdirAll(localPath, 0755); err != nil {
		return fmt.Errorf("failed to create local directory: %w", err)
	}
	result.DirCount++

	entries, err := t.sftp.ReadDir(remotePath)
	if err != nil {
		return fmt.Errorf("failed to read remote directory: %w", err)
	}

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		remoteEntryPath := filepath.Join(remotePath, entry.Name())
		localEntryPath := filepath.Join(localPath, entry.Name())

		if entry.IsDir() {
			if err := t.downloadDirectory(ctx, remoteEntryPath, localEntryPath, opts, result); err != nil {
				return err
			}
		} else {
			if err := t.downloadFile(ctx, remoteEntryPath, localEntryPath, entry, opts, result); err != nil {
				return err
			}
		}
	}

	return nil
}

// ListRemote lists files in a remote directory.
func (t *FileTransfer) ListRemote(path string) ([]os.FileInfo, error) {
	return t.sftp.ReadDir(path)
}

// StatRemote gets file info for a remote path.
func (t *FileTransfer) StatRemote(path string) (os.FileInfo, error) {
	return t.sftp.Stat(path)
}

// MkdirRemote creates a directory on the remote host.
func (t *FileTransfer) MkdirRemote(path string) error {
	return t.sftp.MkdirAll(path)
}

// RemoveRemote removes a file or directory on the remote host.
func (t *FileTransfer) RemoveRemote(path string) error {
	info, err := t.sftp.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return t.removeDirectoryRecursive(path)
	}
	return t.sftp.Remove(path)
}

// removeDirectoryRecursive removes a directory and its contents recursively.
func (t *FileTransfer) removeDirectoryRecursive(path string) error {
	entries, err := t.sftp.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			if err := t.removeDirectoryRecursive(entryPath); err != nil {
				return err
			}
		} else {
			if err := t.sftp.Remove(entryPath); err != nil {
				return err
			}
		}
	}

	return t.sftp.RemoveDirectory(path)
}

// FormatBytes formats bytes to human readable string.
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2fGB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2fMB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2fKB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// FormatSpeed formats speed to human readable string.
func FormatSpeed(bytesPerSecond float64) string {
	return FormatBytes(int64(bytesPerSecond)) + "/s"
}

// FormatProgressBar formats a progress bar string.
func FormatProgressBar(progress *TransferProgress, width int) string {
	if width < 20 {
		width = 20
	}

	barWidth := width - 40 // Leave space for percentage, speed, ETA
	filled := int(progress.Percentage / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("=", filled) + strings.Repeat("-", barWidth-filled)

	return fmt.Sprintf("%s [%s] %.1f%% %s ETA: %s",
		truncateFilename(progress.Filename, 15),
		bar,
		progress.Percentage,
		FormatSpeed(progress.Speed),
		formatDuration(progress.ETA))
}

// truncateFilename truncates a filename if too long.
func truncateFilename(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen-3] + "..."
}

// formatDuration formats a duration to human readable string.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "< 1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
