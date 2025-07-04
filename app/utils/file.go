package utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/h2non/filetype"
)

// Error definitions for file operations
var (
	ErrInvalidPDF      = errors.New("invalid PDF file")
	ErrFileTooLarge    = errors.New("file size exceeds limit")
	ErrInvalidFileType = errors.New("invalid file type")
)

// Constants for file handling configuration
const (
	MaxFileSize     = 10 << 20 // 10MB maximum file size
	PDFMagicNumber  = "%PDF-"  // PDF file signature
	CleanupInterval = 24 * time.Hour // Default expiration time
)

// FileInfo contains metadata for stored files
type FileInfo struct {
	UUID      string    // Unique file identifier
	Path      string    // Full filesystem path
	Name      string    // Original filename
	Size      int64     // File size in bytes
	Sha256    string    // File content hash
	MimeType  string    // Detected MIME type
	CreatedAt time.Time // Creation timestamp
	ExpiresAt time.Time // Scheduled deletion time
}

// ValidatePDF checks if the data contains a valid PDF file
// by verifying both the magic number and MIME type
func ValidatePDF(data []byte) bool {
	// Minimum length check
	if len(data) < len(PDFMagicNumber) {
		return false
	}
	
	// Verify PDF signature and MIME type
	return bytes.HasPrefix(data, []byte(PDFMagicNumber)) &&
		filetype.IsMIME(data, "application/pdf")
}

// SaveTempFile saves data to a temporary file with automatic cleanup
// Returns FileInfo with metadata or error if operation fails
func SaveTempFile(data []byte, prefix string) (*FileInfo, error) {
	// Validate file size limit
	if len(data) > MaxFileSize {
		return nil, ErrFileTooLarge
	}

	// Ensure storage directory exists
	storagePath := filepath.Join("app", "storage", "temp")
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Generate unique filename using UUID
	fileID := uuid.New().String()
	fileName := prefix + "_" + fileID + ".pdf"
	filePath := filepath.Join(storagePath, fileName)

	// Write file contents with restricted permissions
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Calculate SHA-256 checksum for content verification
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// Detect actual MIME type for security validation
	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "application/pdf") {
		os.Remove(filePath) // Clean up invalid file
		return nil, ErrInvalidFileType
	}

	// Prepare file metadata
	fileInfo := &FileInfo{
		UUID:      fileID,
		Path:      filePath,
		Name:      fileName,
		Size:      int64(len(data)),
		Sha256:    hashStr,
		MimeType:  mimeType,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(CleanupInterval),
	}

	// Schedule automatic cleanup
	go scheduleFileDeletion(fileInfo.Path, CleanupInterval)

	return fileInfo, nil
}

// CleanOldFiles removes files older than specified duration
// from the target directory. Useful for maintenance tasks.
func CleanOldFiles(dir string, olderThan time.Duration) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	// Process directory entries
	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip subdirectories
		}

		info, err := entry.Info()
		if err != nil {
			continue // Skip unreadable files
		}

		// Delete expired files
		if time.Since(info.ModTime()) > olderThan {
			filePath := filepath.Join(dir, entry.Name())
			if err := os.Remove(filePath); err != nil {
				log.Printf("Failed to remove old file %s: %v", filePath, err)
			}
		}
	}

	return nil
}

// GetFileExtension returns appropriate file extension
// for given MIME type using system mime database
func GetFileExtension(mimeType string) string {
	exts, err := mime.ExtensionsByType(mimeType)
	if err != nil || len(exts) == 0 {
		return ".bin" // Default extension
	}
	return exts[0] // Return first matching extension
}

// scheduleFileDeletion deletes file after specified duration
// Handles errors silently with logging
func scheduleFileDeletion(filePath string, duration time.Duration) {
	time.Sleep(duration)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		log.Printf("Failed to delete file %s: %v", filePath, err)
	}
}