package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

var logger *logrus.Logger

func init() {
	logger = logrus.New()
	
	// Set log level based on environment
	logLevel := strings.ToLower(getEnv("LOG_LEVEL", "info"))
	switch logLevel {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "info":
		logger.SetLevel(logrus.InfoLevel)
	case "warn", "warning":
		logger.SetLevel(logrus.WarnLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}
	
	// Set formatter based on environment
	if getEnv("LOG_FORMAT", "text") == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
		})
	}
	
	logger.WithFields(logrus.Fields{
		"service": "neorg-documentation-lambda",
		"version": "1.0.0",
		"log_level": logLevel,
	}).Info("Logger initialized")
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

type (
	Response struct {
		Error string `json:"error"`
		Id    string `json:"id"`
	}

	ConversionResult struct {
		Files []string `json:"files"`
		Error string  `json:"error,omitempty"`
		Id    string  `json:"id"`
	}
)

func Unauthorized(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusUnauthorized)
	unauthorized := map[string]any{
		"error": "Unauthorized",
	}
	unauthorizedJson, err := json.Marshal(unauthorized)
	if err != nil {
		logger.WithError(err).Fatal("Failed to create temp file")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}
	w.Write(unauthorizedJson)
}


// Extract tarball and generate documentation using make documentation
func generateDocumentation(ctx context.Context, tarballData []byte, requestId string) (string, error) {
	// Create temporary directory for extraction
	tempDir := fmt.Sprintf("/tmp/neorg_%s", requestId)
	logger.WithFields(logrus.Fields{
		"temp_dir": tempDir,
		"request_id": requestId,
	}).Debug("Creating temporary directory for project extraction")

	err := os.MkdirAll(tempDir, 0755)
	if err != nil {
		logger.WithError(err).Error("Failed to create temporary directory")
		return "", fmt.Errorf("failed to create temp directory: %v", err)
	}

	// Extract tarball to temporary directory
	err = extractTarball(tarballData, tempDir)
	if err != nil {
		logger.WithError(err).Error("Failed to extract tarball")
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to extract tarball: %v", err)
	}

	// Copy docgen files to the project directory
	err = copyDocgenFiles(tempDir)
	if err != nil {
		logger.WithError(err).Error("Failed to copy docgen files")
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to copy docgen files: %v", err)
	}

	// Run make documentation in the project directory
	err = runMakeDocumentation(ctx, tempDir)
	if err != nil {
		logger.WithError(err).Error("Failed to run make documentation")
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to generate documentation: %v", err)
	}

	return tempDir, nil
}

// Extract tarball to specified directory
func extractTarball(tarballData []byte, destDir string) error {
	tarReader := tar.NewReader(bytes.NewReader(tarballData))
	
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar: %v", err)
		}

		targetPath := filepath.Join(destDir, header.Name)
		
		// Ensure the target path is within destDir (security check)
		if !strings.HasPrefix(targetPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(targetPath, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("error creating directory %s: %v", targetPath, err)
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			err = os.MkdirAll(filepath.Dir(targetPath), 0755)
			if err != nil {
				return fmt.Errorf("error creating parent directory for %s: %v", targetPath, err)
			}
			
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("error creating file %s: %v", targetPath, err)
			}
			
			_, err = io.Copy(file, tarReader)
			file.Close()
			if err != nil {
				return fmt.Errorf("error writing file %s: %v", targetPath, err)
			}
		}
	}
	
	return nil
}

// Copy docgen files to the project directory
func copyDocgenFiles(projectDir string) error {
	docgenDir := filepath.Join(projectDir, "docgen")
	err := os.MkdirAll(docgenDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create docgen directory: %v", err)
	}

	// Copy files from ./docgen to projectDir/docgen
	docgenFiles := []string{"init.lua", "docgen.lua", "fileio.lua", "minimal_init.vim", "simple_norg_converter.lua"}
	for _, file := range docgenFiles {
		srcPath := filepath.Join("./docgen", file)
		destPath := filepath.Join(docgenDir, file)
		
		err = copyFile(srcPath, destPath)
		if err != nil {
			return fmt.Errorf("failed to copy %s: %v", file, err)
		}
	}

	// Create Makefile in project directory
	makefilePath := filepath.Join(projectDir, "Makefile")
	makefileContent := `documentation:
	nvim --headless -c "cd ./docgen" -c "source simple_norg_converter.lua" -c 'qa'
`
	err = os.WriteFile(makefilePath, []byte(makefileContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to create Makefile: %v", err)
	}

	return nil
}

// Copy a file from src to dest
func copyFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	return err
}

// Run make documentation in the specified directory
func runMakeDocumentation(ctx context.Context, projectDir string) error {
	logger.WithFields(logrus.Fields{
		"project_dir": projectDir,
	}).Debug("Running make documentation")

	cmd := exec.CommandContext(ctx, "make", "documentation")
	cmd.Dir = projectDir
	
	// Set environment variables for Neovim to find its config and plugins
	cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME=/app",
		"XDG_DATA_HOME=/app/data",
		"HOME=/app",
	)
	
	// Capture command output for debugging
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err := cmd.Run()
	if err != nil {
		logger.WithFields(logrus.Fields{
			"project_dir": projectDir,
			"error":       err.Error(),
			"stdout":      stdout.String(),
			"stderr":      stderr.String(),
		}).Error("Make documentation command failed")
		return err
	}
	
	logger.WithFields(logrus.Fields{
		"project_dir": projectDir,
		"stdout":      stdout.String(),
		"stderr":      stderr.String(),
	}).Info("Make documentation completed successfully")

	return nil
}

// Get the tarball of the neorg project from the request body
func getTarballData(r *http.Request) ([]byte, error) {
	logger.Debug("Reading tarball from request body")
	
	// Read the tarball from the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Error("Failed to read request body")
		return nil, err
	}

	logger.WithFields(logrus.Fields{
		"body_size": len(body),
	}).Debug("Request body read successfully")

	// Basic validation - check if it looks like a tar file
	if len(body) < 512 {
		return nil, fmt.Errorf("file too small to be a valid tarball")
	}

	return body, nil
}


// createZipArchive creates a zip file containing all the generated wiki files
func createZipArchive(wikiDir string, requestId string) (string, error) {
	zipFileName := fmt.Sprintf("documentation_%s.zip", requestId)
	
	logger.WithFields(logrus.Fields{
		"request_id":     requestId,
		"zip_filename":   zipFileName,
		"wiki_dir":       wikiDir,
	}).Info("Creating ZIP archive for generated documentation")

	zipFile, err := os.Create(zipFileName)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"request_id":     requestId,
			"zip_filename":   zipFileName,
			"error":          err.Error(),
		}).Error("Failed to create ZIP file")
		return "", err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Walk through the wiki directory and add all files to the zip
	var totalBytesAdded int64
	var fileCount int
	
	err = filepath.Walk(wikiDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip directories
		if info.IsDir() {
			return nil
		}
		
		// Get relative path from wiki directory
		relPath, err := filepath.Rel(wikiDir, path)
		if err != nil {
			return err
		}
		
		logger.WithFields(logrus.Fields{
			"request_id":      requestId,
			"file_path":       path,
			"relative_path":   relPath,
			"file_size":       info.Size(),
		}).Debug("Adding file to ZIP archive")

		// Open the file
		file, err := os.Open(path)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"request_id": requestId,
				"file_path":  path,
				"error":      err.Error(),
			}).Error("Failed to open file for ZIP archive")
			return err
		}
		defer file.Close()

		// Create a file header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"request_id": requestId,
				"file_path":  path,
				"error":      err.Error(),
			}).Error("Failed to create ZIP header for file")
			return err
		}

		// Use the relative path as the name in the zip
		header.Name = relPath
		header.Method = zip.Deflate

		// Create the file in the zip
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"request_id": requestId,
				"file_path":  path,
				"zip_entry":  header.Name,
				"error":      err.Error(),
			}).Error("Failed to create ZIP entry for file")
			return err
		}

		// Copy the file content to the zip
		bytesWritten, err := io.Copy(writer, file)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"request_id": requestId,
				"file_path":  path,
				"zip_entry":  header.Name,
				"error":      err.Error(),
			}).Error("Failed to write file content to ZIP")
			return err
		}

		totalBytesAdded += bytesWritten
		fileCount++
		
		logger.WithFields(logrus.Fields{
			"request_id":     requestId,
			"file_path":      path,
			"zip_entry":      header.Name,
			"bytes_written":  bytesWritten,
		}).Debug("File successfully added to ZIP archive")

		return nil
	})
	
	if err != nil {
		return "", fmt.Errorf("failed to walk wiki directory: %v", err)
	}

	logger.WithFields(logrus.Fields{
		"request_id":        requestId,
		"zip_filename":      zipFileName,
		"files_added":       fileCount,
		"total_bytes_added": totalBytesAdded,
	}).Info("ZIP archive creation completed successfully")

	return zipFileName, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	requestId := uuid.New().String()
	// Request logging is now handled by middleware, but we'll keep request ID for internal tracking
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("request-id", requestId)

	// Check authentication
	AuthTokenHeader := r.Header.Get("x-auth-token")
	expectedToken := getEnv("NEORG_DOCUMENTATION_AUTH_TOKEN", "")
	if expectedToken == "" || AuthTokenHeader != expectedToken {
		Unauthorized(w, r)
		return
	}

	// Only allow POST method
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(Response{
			Error: "Method not allowed",
			Id:    requestId,
		})
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Get the tarball data from the request body
	tarballData, err := getTarballData(r)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
			"error": err.Error(),
		}).Error("Failed to get tarball from request")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Error: "Failed to process tarball",
			Id:    requestId,
		})
		return
	}

	logger.WithFields(logrus.Fields{
		"request_id": requestId,
		"tarball_size": len(tarballData),
	}).Info("Starting documentation generation")

	// Generate documentation using the Neorg approach
	projectDir, err := generateDocumentation(ctx, tarballData, requestId)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
			"error": err.Error(),
		}).Error("Failed to generate documentation")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Error: fmt.Sprintf("Documentation generation failed: %v", err),
			Id:    requestId,
		})
		return
	}

	// Clean up project directory when done
	defer os.RemoveAll(projectDir)

	// Check if wiki directory was created
	wikiDir := filepath.Join(projectDir, "wiki")
	if _, err := os.Stat(wikiDir); os.IsNotExist(err) {
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
			"wiki_dir": wikiDir,
		}).Error("Wiki directory was not created - documentation generation may have failed")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Error: "No documentation was generated",
			Id:    requestId,
		})
		return
	}

	// Create zip archive of generated documentation
	zipFileName, err := createZipArchive(wikiDir, requestId)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
			"error": err.Error(),
		}).Error("Failed to create output zip archive")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Error: fmt.Sprintf("Failed to create zip archive: %v", err),
			Id:    requestId,
		})
		return
	}

	// Clean up zip file after response
	defer os.Remove(zipFileName)

	// Open the zip file for reading
	zipFile, err := os.Open(zipFileName)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
			"error": err.Error(),
		}).Error("Failed to open created zip file for reading")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Error: "Failed to open zip file",
			Id:    requestId,
		})
		return
	}
	defer zipFile.Close()

	// Get file info for content length
	zipInfo, err := zipFile.Stat()
	if err != nil {
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
			"error": err.Error(),
		}).Error("Failed to get zip file information")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Error: "Failed to get zip file info",
			Id:    requestId,
		})
		return
	}

	// Set response headers for file download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"neorg_documentation_%s.zip\"", requestId))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", zipInfo.Size()))
	w.Header().Set("request-id", requestId)

	// Stream the zip file to the response
	logger.WithFields(logrus.Fields{
		"request_id": requestId,
		"zip_size_bytes": zipInfo.Size(),
	}).Info("Successfully generated documentation, sending response")
	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, zipFile)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
			"error": err.Error(),
		}).Error("Failed to stream zip file to client")
	}
}

// LoggingMiddleware wraps HTTP handlers with comprehensive logging
func LoggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestId := uuid.New().String()
		
		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		w.Header().Set("request-id", requestId)
		
		// Log request start
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
			"method":     r.Method,
			"path":       r.URL.Path,
			"query":      r.URL.RawQuery,
			"remote_addr": r.RemoteAddr,
			"user_agent":  r.UserAgent(),
			"content_length": r.ContentLength,
		}).Info("Request started")
		
		// Call the next handler
		next(wrapped, r)
		
		// Log request completion
		duration := time.Since(start)
		logLevel := logrus.InfoLevel
		if wrapped.statusCode >= 400 {
			logLevel = logrus.WarnLevel
		}
		if wrapped.statusCode >= 500 {
			logLevel = logrus.ErrorLevel
		}
		
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
			"method":     r.Method,
			"path":       r.URL.Path,
			"status_code": wrapped.statusCode,
			"duration_ms": duration.Milliseconds(),
			"bytes_written": wrapped.bytesWritten,
		}).Log(logLevel, "Request completed")
	}
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes written
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// checkNeorgHealth runs a simple test to verify Neovim and make are available
func checkNeorgHealth() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Check if nvim is available
	cmd := exec.CommandContext(ctx, "/opt/nvim/bin/nvim", "--version")
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("nvim not available: %v", err)
	}
	
	// Check if make is available
	cmd = exec.CommandContext(ctx, "make", "--version")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("make not available: %v", err)
	}
	
	// Check if docgen files exist
	docgenFiles := []string{"./docgen/init.lua", "./docgen/docgen.lua", "./docgen/fileio.lua", "./docgen/minimal_init.vim"}
	for _, file := range docgenFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			return fmt.Errorf("docgen file missing: %s", file)
		}
	}
	
	return nil
}

func check_health(w http.ResponseWriter, r *http.Request) {
	logger.WithFields(logrus.Fields{
		"endpoint": "/health",
		"method":   r.Method,
	}).Debug("Health check requested")
	
	// Check Neorg health
	if err := checkNeorgHealth(); err != nil {
		logger.WithError(err).Error("Neorg health check failed")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "Service unavailable: Neorg not ready - %v", err)
		return
	}
	
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
	
	logger.Debug("Health check completed successfully")
}

func main() {
	port := getEnv("PORT", "8080")
	logger.WithFields(logrus.Fields{
		"service": "neorg-documentation-lambda",
		"port":    "8080",
	}).Info("Starting Neorg Documentation Lambda server")
	
	// Wrap handlers with logging middleware
	http.HandleFunc("/", LoggingMiddleware(handler))
	http.HandleFunc("/health", LoggingMiddleware(check_health))
	
	logger.Info("Server routes registered, starting HTTP server on port " + port)
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.WithFields(logrus.Fields{
			"error": err.Error(),
			"port":  "8080",
		}).Fatal("Failed to start HTTP server")
	}
}
