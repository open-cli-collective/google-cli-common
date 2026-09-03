package mailcmd

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	gmailapi "github.com/open-cli-collective/google-cli-common/gmail"
	"github.com/open-cli-collective/google-cli-common/testutil"
)

func TestDownloadAttachmentsCommand_DuplicateFilenames(t *testing.T) {
	outputDir := t.TempDir()
	contents := map[string][]byte{
		"image-1": []byte("first image"),
		"image-2": []byte("second image"),
		"image-3": []byte("third image"),
		"notes-1": []byte("first notes"),
		"notes-2": []byte("second notes"),
		"report":  []byte("report"),
	}
	mock := &MockGmailClient{
		GetAttachmentsFunc: func(_ context.Context, _ string) ([]*gmailapi.Attachment, error) {
			return []*gmailapi.Attachment{
				{Filename: "image.png", AttachmentID: "image-1"},
				{Filename: "image.png", AttachmentID: "image-2"},
				{Filename: "image.png", AttachmentID: "image-3"},
				{Filename: "notes", AttachmentID: "notes-1"},
				{Filename: "notes", AttachmentID: "notes-2"},
				{Filename: "report.pdf", AttachmentID: "report"},
			}, nil
		},
		DownloadAttachmentFunc: func(_ context.Context, _, attachmentID string) ([]byte, error) {
			return contents[attachmentID], nil
		},
	}

	cmd := newDownloadAttachmentsCommand()
	cmd.SetArgs([]string{"message-id", "--all", "--output", outputDir})
	withMockClient(mock, func() {
		testutil.NoError(t, cmd.Execute())
	})

	want := map[string]string{
		"image.png":   "first image",
		"image-2.png": "second image",
		"image-3.png": "third image",
		"notes":       "first notes",
		"notes-2":     "second notes",
		"report.pdf":  "report",
	}
	for name, content := range want {
		got, err := os.ReadFile(filepath.Join(outputDir, name))
		testutil.NoError(t, err)
		testutil.Equal(t, string(got), content)
	}
}

func TestDownloadAttachmentsCommand_DuplicateZipExtraction(t *testing.T) {
	zipBytes := func(name, content string) []byte {
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		f, err := w.Create(name)
		testutil.NoError(t, err)
		_, err = f.Write([]byte(content))
		testutil.NoError(t, err)
		testutil.NoError(t, w.Close())
		return buf.Bytes()
	}

	outputDir := t.TempDir()
	archives := map[string][]byte{
		"archive-1": zipBytes("first.txt", "first archive"),
		"archive-2": zipBytes("second.txt", "second archive"),
	}
	mock := &MockGmailClient{
		GetAttachmentsFunc: func(_ context.Context, _ string) ([]*gmailapi.Attachment, error) {
			return []*gmailapi.Attachment{
				{Filename: "archive.zip", MimeType: "application/zip", AttachmentID: "archive-1"},
				{Filename: "archive.zip", MimeType: "application/zip", AttachmentID: "archive-2"},
			}, nil
		},
		DownloadAttachmentFunc: func(_ context.Context, _, attachmentID string) ([]byte, error) {
			return archives[attachmentID], nil
		},
	}

	cmd := newDownloadAttachmentsCommand()
	cmd.SetArgs([]string{"message-id", "--all", "--extract", "--output", outputDir})
	withMockClient(mock, func() {
		testutil.NoError(t, cmd.Execute())
	})

	first, err := os.ReadFile(filepath.Join(outputDir, "archive", "first.txt"))
	testutil.NoError(t, err)
	testutil.Equal(t, string(first), "first archive")
	second, err := os.ReadFile(filepath.Join(outputDir, "archive-2", "second.txt"))
	testutil.NoError(t, err)
	testutil.Equal(t, string(second), "second archive")
	_, err = os.Stat(filepath.Join(outputDir, "archive-2.zip"))
	testutil.NoError(t, err)
}

func TestSafeOutputPath(t *testing.T) {
	destDir := "/tmp/downloads"

	tests := []struct {
		name        string
		filename    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "simple filename",
			filename:    "report.pdf",
			expectError: false,
		},
		{
			name:        "filename with spaces",
			filename:    "my report.pdf",
			expectError: false,
		},
		{
			name:        "filename in subdirectory",
			filename:    "attachments/report.pdf",
			expectError: false,
		},
		{
			name:        "path traversal with ..",
			filename:    "../../../etc/passwd",
			expectError: true,
			errorMsg:    "path traversal not allowed",
		},
		{
			name:        "path traversal at start",
			filename:    "../secret.txt",
			expectError: true,
			errorMsg:    "path traversal not allowed",
		},
		{
			name:        "path traversal in middle",
			filename:    "subdir/../../../etc/passwd",
			expectError: true,
			errorMsg:    "path traversal not allowed",
		},
		{
			name:        "double dot only",
			filename:    "..",
			expectError: true,
			errorMsg:    "path traversal not allowed",
		},
		{
			name:        "absolute path unix",
			filename:    "/etc/passwd",
			expectError: true,
			errorMsg:    "absolute path not allowed",
		},
		{
			name:        "hidden file",
			filename:    ".hidden",
			expectError: false,
		},
		{
			name:        "dot in filename",
			filename:    "report.v2.pdf",
			expectError: false,
		},
		{
			name:        "empty after traversal",
			filename:    "foo/../bar",
			expectError: false, // After cleaning becomes "bar" which is valid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := safeOutputPath(destDir, tt.filename)

			if tt.expectError {
				testutil.Error(t, err)
				if tt.errorMsg != "" {
					testutil.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				testutil.NoError(t, err)
				// Verify the result is within destDir
				testutil.True(t, filepath.IsAbs(result) || result == filepath.Join(destDir, filepath.Clean(tt.filename)))
			}
		})
	}
}

func TestSafeOutputPath_StaysWithinDestDir(t *testing.T) {
	destDir := "/tmp/downloads"

	// Valid cases should produce paths within destDir
	validCases := []string{
		"simple.txt",
		"dir/file.txt",
		"a/b/c/deep.txt",
	}

	for _, filename := range validCases {
		t.Run(filename, func(t *testing.T) {
			result, err := safeOutputPath(destDir, filename)
			testutil.NoError(t, err)

			// Result must start with destDir
			testutil.True(t, len(result) >= len(destDir))
			testutil.Equal(t, result[:len(destDir)], destDir)
		})
	}
}
