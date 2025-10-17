package main

import (
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
	"sync"
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

	ConversionJob struct {
		InputFile  *os.File
		OutputFile *os.File
		Error      error
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

const ()

func convertArgs(inputFileName, outputFileName *os.File) []string {
	return []string{
		inputFileName.Name(),
		"--headless",
		"-c", fmt.Sprintf(":Neorg export to-file %s markdown", outputFileName.Name()),
		"-c", ":q",
	}
}

// Convert neorg to markdown using the neovim configuration in the container
func runNeorgConvert(ctx context.Context, inputFile *os.File) (*os.File, error) {
	// read the filename and remove the .norg extension
	filename := filepath.Base(inputFile.Name())
	originalFilename := filename
	filename = strings.TrimSuffix(filename, ".norg")
	outputFilename := filename + ".md"

	logger.WithFields(logrus.Fields{
		"input_file":  originalFilename,
		"output_file": outputFilename,
	}).Debug("Starting Neorg conversion")

	// Create an output file in the current directory
	outputFile, err := os.Create(outputFilename)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"output_file": outputFilename,
			"error":       err.Error(),
		}).Error("Failed to create output file")
		return nil, err
	}

	// Run the neorg command with context timeout
	args := convertArgs(inputFile, outputFile)
	cmd := exec.CommandContext(ctx, "/opt/nvim/bin/nvim", args...)
	
	logger.WithFields(logrus.Fields{
		"input_file":  originalFilename,
		"output_file": outputFilename,
		"command":     "nvim",
		"args":        args,
	}).Debug("Executing Neovim conversion command")

	// Capture command output for debugging
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err = cmd.Run()
	if err != nil {
		logger.WithFields(logrus.Fields{
			"input_file":  originalFilename,
			"output_file": outputFilename,
			"error":       err.Error(),
			"command":     fmt.Sprintf("nvim %v", args),
			"stdout":      stdout.String(),
			"stderr":      stderr.String(),
		}).Error("Neovim conversion command failed")
		
		outputFile.Close()
		os.Remove(outputFile.Name())
		return nil, err
	}
	
	// Log command output for debugging
	logger.WithFields(logrus.Fields{
		"input_file":  originalFilename,
		"output_file": outputFilename,
		"stdout":      stdout.String(),
		"stderr":      stderr.String(),
	}).Debug("Neovim command output")

	// Check if output file has content
	outputFile.Seek(0, 0) // Seek back to beginning for reading
	outputFileInfo, _ := outputFile.Stat()
	fileSize := outputFileInfo.Size()
	
	// Read and log the actual file contents for debugging
	fileContents, err := io.ReadAll(outputFile)
	if err != nil {
		logger.WithError(err).Error("Failed to read output file contents")
	} else {
		logger.WithFields(logrus.Fields{
			"input_file":     originalFilename,
			"output_file":    outputFilename,
			"output_size":    fileSize,
			"file_contents":  string(fileContents),
			"contents_length": len(fileContents),
		}).Info("Neorg conversion completed - file contents")
	}
	
	// Reset file pointer for later use
	outputFile.Seek(0, 0)
	
	logger.WithFields(logrus.Fields{
		"input_file":    originalFilename,
		"output_file":   outputFilename,
		"output_size":   fileSize,
	}).Info("Neorg conversion completed successfully")

	return outputFile, nil
}

// Get the zip archive of the neorg documents
// from the body of the request
// and unzip it as an array of files
func getNeorgDocuments(r *http.Request) ([]*os.File, error) {
	logger.Debug("Starting to extract Neorg documents from request body")
	
	// Read the zip from the request body
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

	// Unzip the archive
	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		logger.WithFields(logrus.Fields{
			"error":     err.Error(),
			"body_size": len(body),
		}).Error("Failed to create zip reader from request body")
		return nil, err
	}

	logger.WithFields(logrus.Fields{
		"total_files": len(zipReader.File),
	}).Debug("Zip archive opened successfully")

	// Read the zip file and return the files
	var files []*os.File
	var processedFiles, skippedFiles int

	for _, file := range zipReader.File {
		// Only process .norg files
		if !strings.HasSuffix(file.Name, ".norg") {
			logger.WithFields(logrus.Fields{
				"filename": file.Name,
			}).Debug("Skipping non-.norg file")
			skippedFiles++
			continue
		}

		logger.WithFields(logrus.Fields{
			"filename": file.Name,
			"size":     file.UncompressedSize64,
		}).Debug("Processing .norg file from archive")

		f, err := file.Open()
		if err != nil {
			logger.WithFields(logrus.Fields{
				"filename": file.Name,
				"error":    err.Error(),
			}).Error("Failed to open file from zip archive")
			return nil, err
		}
		defer f.Close()

		// Create unique filename to avoid conflicts
		uniqueID := uuid.New().String()[:8]
		filename := fmt.Sprintf("%s_%s", uniqueID, filepath.Base(file.Name))
		
		logger.WithFields(logrus.Fields{
			"original_filename": file.Name,
			"temp_filename":     filename,
			"unique_id":         uniqueID,
		}).Debug("Creating temporary file for processing")

		fileUnzipped, err := os.Create(filename)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"filename": filename,
				"error":    err.Error(),
			}).Error("Failed to create temporary file")
			return nil, err
		}

		bytesWritten, err := io.Copy(fileUnzipped, f)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"filename": filename,
				"error":    err.Error(),
			}).Error("Failed to write file contents to temporary file")
			fileUnzipped.Close()
			os.Remove(fileUnzipped.Name())
			return nil, err
		}

		logger.WithFields(logrus.Fields{
			"original_filename": file.Name,
			"temp_filename":     filename,
			"bytes_written":     bytesWritten,
		}).Debug("File extracted successfully to temporary location")

		// Seek to beginning for reading
		fileUnzipped.Seek(0, 0)
		files = append(files, fileUnzipped)
		processedFiles++
	}

	logger.WithFields(logrus.Fields{
		"total_files":     len(zipReader.File),
		"processed_files": processedFiles,
		"skipped_files":   skippedFiles,
		"norg_files":      len(files),
	}).Info("Finished extracting Neorg documents from archive")

	return files, nil
}

func processFiles(ctx context.Context, files []*os.File, maxWorkers int) ([]string, error) {
	logger.WithFields(logrus.Fields{
		"file_count":   len(files),
		"worker_count": maxWorkers,
	}).Info("Initializing worker pool for file processing")

	jobs := make(chan *os.File, len(files))
	results := make(chan ConversionJob, len(files))
	startTime := time.Now()

	// Start worker pool
	var wg sync.WaitGroup
	for i := range maxWorkers {
		wg.Add(1)
		workerID := i + 1
		go func() {
			defer wg.Done()
			logger.WithFields(logrus.Fields{
				"worker_id": workerID,
			}).Debug("Worker started")
			
			jobsProcessed := 0
			for inputFile := range jobs {
				select {
				case <-ctx.Done():
					logger.WithFields(logrus.Fields{
						"worker_id":      workerID,
						"jobs_processed": jobsProcessed,
					}).Warn("Worker stopped due to context cancellation")
					return
				default:
					logger.WithFields(logrus.Fields{
						"worker_id":  workerID,
						"input_file": inputFile.Name(),
					}).Debug("Worker processing file")
					
					outputFile, err := runNeorgConvert(ctx, inputFile)
					results <- ConversionJob{
						InputFile:  inputFile,
						OutputFile: outputFile,
						Error:      err,
					}
					jobsProcessed++
				}
			}
			
			logger.WithFields(logrus.Fields{
				"worker_id":      workerID,
				"jobs_processed": jobsProcessed,
			}).Debug("Worker finished processing all jobs")
		}()
	}

	logger.WithFields(logrus.Fields{
		"worker_count": maxWorkers,
	}).Debug("All workers started, sending jobs to queue")

	// Send jobs
	go func() {
		defer close(jobs)
		for i, file := range files {
			select {
			case jobs <- file:
				logger.WithFields(logrus.Fields{
					"file_index": i + 1,
					"total_files": len(files),
					"filename": file.Name(),
				}).Debug("File queued for processing")
			case <-ctx.Done():
				logger.WithFields(logrus.Fields{
					"files_queued": i,
					"total_files": len(files),
				}).Warn("Job queuing stopped due to context cancellation")
				return
			}
		}
		logger.WithFields(logrus.Fields{
			"total_files": len(files),
		}).Debug("All files queued for processing")
	}()

	// Close results channel when all workers are done
	go func() {
		wg.Wait()
		logger.Debug("All workers completed, closing results channel")
		close(results)
	}()

	// Collect results
	var outputFiles []string
	var conversionErrors []string
	processedCount := 0

	logger.Debug("Starting to collect conversion results")
	for job := range results {
		processedCount++
		
		// Clean up input file
		if job.InputFile != nil {
			inputFileName := job.InputFile.Name()
			job.InputFile.Close()
			os.Remove(job.InputFile.Name())
			
			if job.Error != nil {
				logger.WithFields(logrus.Fields{
					"input_file": inputFileName,
					"error":      job.Error.Error(),
					"processed":  processedCount,
					"total":      len(files),
				}).Warn("File conversion failed")
				conversionErrors = append(conversionErrors, fmt.Sprintf("Error converting %s: %v", inputFileName, job.Error))
				continue
			}

			if job.OutputFile != nil {
				outputFileName := job.OutputFile.Name()
				outputFiles = append(outputFiles, outputFileName)
				job.OutputFile.Close()
				
				logger.WithFields(logrus.Fields{
					"input_file":  inputFileName,
					"output_file": outputFileName,
					"processed":   processedCount,
					"total":       len(files),
				}).Debug("File conversion completed successfully")
			} else {
				logger.WithFields(logrus.Fields{
					"input_file": inputFileName,
					"processed":  processedCount,
					"total":      len(files),
				}).Warn("File conversion completed but no output file generated")
			}
		}
	}

	duration := time.Since(startTime)
	logger.WithFields(logrus.Fields{
		"total_files":        len(files),
		"successful_conversions": len(outputFiles),
		"failed_conversions": len(conversionErrors),
		"duration_ms":        duration.Milliseconds(),
		"files_per_second":   float64(len(files)) / duration.Seconds(),
	}).Info("File processing completed")

	if len(conversionErrors) > 0 {
		logger.WithFields(logrus.Fields{
			"conversion_errors": conversionErrors,
			"error_count":       len(conversionErrors),
		}).Error("Some files failed to convert")
		return outputFiles, fmt.Errorf("conversion errors: %s", strings.Join(conversionErrors, "; "))
	}

	logger.WithFields(logrus.Fields{
		"output_files": outputFiles,
	}).Debug("All files converted successfully")

	return outputFiles, nil
}

// createZipArchive creates a zip file containing all the converted markdown files
func createZipArchive(outputFiles []string, requestId string) (string, error) {
	zipFileName := fmt.Sprintf("converted_%s.zip", requestId)
	
	logger.WithFields(logrus.Fields{
		"request_id":     requestId,
		"zip_filename":   zipFileName,
		"file_count":     len(outputFiles),
		"output_files":   outputFiles,
	}).Info("Creating ZIP archive for converted files")

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

	var totalBytesAdded int64
	for i, fileName := range outputFiles {
		logger.WithFields(logrus.Fields{
			"request_id":      requestId,
			"file_index":      i + 1,
			"total_files":     len(outputFiles),
			"processing_file": fileName,
		}).Debug("Adding file to ZIP archive")

		// Read the converted markdown file
		markdownFile, err := os.Open(fileName)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"request_id": requestId,
				"filename":   fileName,
				"error":      err.Error(),
			}).Error("Failed to open converted file for ZIP archive")
			return "", fmt.Errorf("failed to open %s: %v", fileName, err)
		}

		// Get file info for the header
		info, err := markdownFile.Stat()
		if err != nil {
			markdownFile.Close()
			logger.WithFields(logrus.Fields{
				"request_id": requestId,
				"filename":   fileName,
				"error":      err.Error(),
			}).Error("Failed to get file info for ZIP archive")
			return "", fmt.Errorf("failed to stat %s: %v", fileName, err)
		}

		// Create a file header based on the file info
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			markdownFile.Close()
			logger.WithFields(logrus.Fields{
				"request_id": requestId,
				"filename":   fileName,
				"error":      err.Error(),
			}).Error("Failed to create ZIP header for file")
			return "", fmt.Errorf("failed to create header for %s: %v", fileName, err)
		}

		// Use only the base filename in the zip (remove any path prefixes)
		header.Name = filepath.Base(fileName)
		header.Method = zip.Deflate

		logger.WithFields(logrus.Fields{
			"request_id":    requestId,
			"filename":      fileName,
			"zip_entry":     header.Name,
			"file_size":     info.Size(),
		}).Debug("Creating ZIP entry for file")

		// Create the file in the zip
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			markdownFile.Close()
			logger.WithFields(logrus.Fields{
				"request_id": requestId,
				"filename":   fileName,
				"zip_entry":  header.Name,
				"error":      err.Error(),
			}).Error("Failed to create ZIP entry for file")
			return "", fmt.Errorf("failed to create zip entry for %s: %v", fileName, err)
		}

		// Copy the file content to the zip
		bytesWritten, err := io.Copy(writer, markdownFile)
		markdownFile.Close()
		if err != nil {
			logger.WithFields(logrus.Fields{
				"request_id": requestId,
				"filename":   fileName,
				"zip_entry":  header.Name,
				"error":      err.Error(),
			}).Error("Failed to write file content to ZIP")
			return "", fmt.Errorf("failed to write %s to zip: %v", fileName, err)
		}

		totalBytesAdded += bytesWritten
		logger.WithFields(logrus.Fields{
			"request_id":     requestId,
			"filename":       fileName,
			"zip_entry":      header.Name,
			"bytes_written":  bytesWritten,
			"file_index":     i + 1,
			"total_files":    len(outputFiles),
		}).Debug("File successfully added to ZIP archive")

		// Clean up the temporary markdown file
		os.Remove(fileName)
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
			"filename":   fileName,
		}).Debug("Temporary file cleaned up")
	}

	logger.WithFields(logrus.Fields{
		"request_id":        requestId,
		"zip_filename":      zipFileName,
		"files_added":       len(outputFiles),
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

	// Get the neorg documents from the request body
	neorgDocuments, err := getNeorgDocuments(r)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
			"error": err.Error(),
		}).Error("Failed to get neorg documents from request")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Error: "Failed to process zip file",
			Id:    requestId,
		})
		return
	}

	if len(neorgDocuments) == 0 {
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
		}).Warn("No .norg files found in uploaded zip archive")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Error: "No .norg files found in uploaded zip",
			Id:    requestId,
		})
		return
	}

	// Process files with worker pool (max 3 concurrent conversions)
	maxWorkers := min(len(neorgDocuments), 3)

	logger.WithFields(logrus.Fields{
		"request_id": requestId,
		"file_count": len(neorgDocuments),
		"worker_count": maxWorkers,
	}).Info("Starting file processing with worker pool")
	outputFiles, err := processFiles(ctx, neorgDocuments, maxWorkers)

	if err != nil {
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
			"error": err.Error(),
		}).Error("Failed to process files with worker pool")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Error: fmt.Sprintf("Conversion failed: %v", err),
			Id:    requestId,
		})
		return
	}

	if len(outputFiles) == 0 {
		logger.WithFields(logrus.Fields{
			"request_id": requestId,
		}).Error("No files were converted successfully - check Neovim configuration")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Error: "No files were converted successfully",
			Id:    requestId,
		})
		return
	}

	// Create zip archive of converted files
	zipFileName, err := createZipArchive(outputFiles, requestId)
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

	// Clean up individual output files and zip file after response
	defer func() {
		for _, file := range outputFiles {
			os.Remove(file)
		}
		os.Remove(zipFileName)
	}()

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
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"converted_markdown_%s.zip\"", requestId))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", zipInfo.Size()))
	w.Header().Set("request-id", requestId)

	// Stream the zip file to the response
	logger.WithFields(logrus.Fields{
		"request_id": requestId,
		"converted_files": len(outputFiles),
		"zip_size_bytes": zipInfo.Size(),
	}).Info("Successfully converted files, sending response")
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

// checkNeorgHealth runs Neovim health check for Neorg functionality
func checkNeorgHealth() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, "/opt/nvim/bin/nvim", "--headless", "-c", ":checkhealth neorg", "-c", ":q")
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err := cmd.Run()
	
	// Log the health check output
	logger.WithFields(logrus.Fields{
		"stdout": stdout.String(),
		"stderr": stderr.String(),
		"error":  err,
	}).Debug("Neorg health check output")
	
	// Check for specific error indicators in the output
	output := stdout.String() + stderr.String()
	if strings.Contains(output, "ERROR") || strings.Contains(output, "command is currently disabled") {
		return fmt.Errorf("neorg health check failed: %s", output)
	}
	
	return err
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
	logger.WithFields(logrus.Fields{
		"service": "neorg-documentation-lambda",
		"port":    "8080",
	}).Info("Starting Neorg Documentation Lambda server")
	
	// Wrap handlers with logging middleware
	http.HandleFunc("/", LoggingMiddleware(handler))
	http.HandleFunc("/health", LoggingMiddleware(check_health))
	
	logger.Info("Server routes registered, starting HTTP server on :8080")
	
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logger.WithFields(logrus.Fields{
			"error": err.Error(),
			"port":  "8080",
		}).Fatal("Failed to start HTTP server")
	}
}
