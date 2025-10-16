package serverless

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

func init() {
	log.SetFlags(log.Lshortfile)
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
		log.Fatal(err)
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
		"--noplugin",
		"-u", "/app/.config/nvim/init.lua",
		"-c", fmt.Sprintf(":Neorg export to-file %s markdown", outputFileName.Name()),
		"-c", ":q",
	}
}

// Convert neorg to markdown using the neovim configuration in the container
func runNeorgConvert(ctx context.Context, inputFile *os.File) (*os.File, error) {
	// read the filename and remove the .norg extension
	filename := filepath.Base(inputFile.Name())
	filename = strings.TrimSuffix(filename, ".norg")

	// Create an output file in the current directory
	outputFile, err := os.Create(filename + ".md")
	if err != nil {
		return nil, err
	}

	// Run the neorg command with context timeout
	cmd := exec.CommandContext(ctx, "nvim", convertArgs(inputFile, outputFile)...)
	err = cmd.Run()
	if err != nil {
		outputFile.Close()
		os.Remove(outputFile.Name())
		return nil, err
	}

	// Seek to beginning for reading
	outputFile.Seek(0, 0)
	return outputFile, nil
}

// Get the zip archive of the neorg documents
// from the body of the request
// and unzip it as an array of files
func getNeorgDocuments(r *http.Request) ([]*os.File, error) {
	// Read the zip from the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	// Unzip the archive
	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, err
	}

	// Read the zip file and return the files
	var files []*os.File
	for _, file := range zipReader.File {
		// Only process .norg files
		if !strings.HasSuffix(file.Name, ".norg") {
			continue
		}

		f, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer f.Close()

		// Create unique filename to avoid conflicts
		uniqueID := uuid.New().String()[:8]
		filename := fmt.Sprintf("%s_%s", uniqueID, filepath.Base(file.Name))
		fileUnzipped, err := os.Create(filename)
		if err != nil {
			return nil, err
		}

		_, err = io.Copy(fileUnzipped, f)
		if err != nil {
			fileUnzipped.Close()
			os.Remove(fileUnzipped.Name())
			return nil, err
		}

		// Seek to beginning for reading
		fileUnzipped.Seek(0, 0)
		files = append(files, fileUnzipped)
	}
	return files, nil
}

func processFiles(ctx context.Context, files []*os.File, maxWorkers int) ([]string, error) {
	jobs := make(chan *os.File, len(files))
	results := make(chan ConversionJob, len(files))

	// Start worker pool
	var wg sync.WaitGroup
	for range maxWorkers {
		wg.Go(func() {
			for inputFile := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
					outputFile, err := runNeorgConvert(ctx, inputFile)
					results <- ConversionJob{
						InputFile:  inputFile,
						OutputFile: outputFile,
						Error:      err,
					}
				}
			}
		})
	}

	// Send jobs
	go func() {
		defer close(jobs)
		for _, file := range files {
			select {
			case jobs <- file:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Close results channel when all workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var outputFiles []string
	var conversionErrors []string

	for job := range results {
		// Clean up input file
		if job.InputFile != nil {
			job.InputFile.Close()
			os.Remove(job.InputFile.Name())
		}

		if job.Error != nil {
			conversionErrors = append(conversionErrors, fmt.Sprintf("Error converting %s: %v", job.InputFile.Name(), job.Error))
			continue
		}

		if job.OutputFile != nil {
			outputFiles = append(outputFiles, job.OutputFile.Name())
			job.OutputFile.Close()
		}
	}

	if len(conversionErrors) > 0 {
		return outputFiles, fmt.Errorf("conversion errors: %s", strings.Join(conversionErrors, "; "))
	}

	return outputFiles, nil
}

// createZipArchive creates a zip file containing all the converted markdown files
func createZipArchive(outputFiles []string, requestId string) (string, error) {
	zipFileName := fmt.Sprintf("converted_%s.zip", requestId)
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		return "", err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, fileName := range outputFiles {
		// Read the converted markdown file
		markdownFile, err := os.Open(fileName)
		if err != nil {
			return "", fmt.Errorf("failed to open %s: %v", fileName, err)
		}

		// Get file info for the header
		info, err := markdownFile.Stat()
		if err != nil {
			markdownFile.Close()
			return "", fmt.Errorf("failed to stat %s: %v", fileName, err)
		}

		// Create a file header based on the file info
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			markdownFile.Close()
			return "", fmt.Errorf("failed to create header for %s: %v", fileName, err)
		}

		// Use only the base filename in the zip (remove any path prefixes)
		header.Name = filepath.Base(fileName)
		header.Method = zip.Deflate

		// Create the file in the zip
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			markdownFile.Close()
			return "", fmt.Errorf("failed to create zip entry for %s: %v", fileName, err)
		}

		// Copy the file content to the zip
		_, err = io.Copy(writer, markdownFile)
		markdownFile.Close()
		if err != nil {
			return "", fmt.Errorf("failed to write %s to zip: %v", fileName, err)
		}
	}

	return zipFileName, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	requestId := uuid.New().String()
	log.Printf("[%s] %s %s", requestId, r.Method, r.URL)
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
		log.Printf("[%s] Error getting neorg documents: %v", requestId, err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Error: "Failed to process zip file",
			Id:    requestId,
		})
		return
	}

	if len(neorgDocuments) == 0 {
		log.Printf("[%s] No .norg files found in zip", requestId)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Error: "No .norg files found in uploaded zip",
			Id:    requestId,
		})
		return
	}

	// Process files with worker pool (max 3 concurrent conversions)
	maxWorkers := min(len(neorgDocuments), 3)

	log.Printf("[%s] Processing %d files with %d workers", requestId, len(neorgDocuments), maxWorkers)
	outputFiles, err := processFiles(ctx, neorgDocuments, maxWorkers)

	if err != nil {
		log.Printf("[%s] Error processing files: %v", requestId, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{
			Error: fmt.Sprintf("Conversion failed: %v", err),
			Id:    requestId,
		})
		return
	}

	if len(outputFiles) == 0 {
		log.Printf("[%s] No files were converted successfully", requestId)
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
		log.Printf("[%s] Error creating zip archive: %v", requestId, err)
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
		log.Printf("[%s] Error opening zip file: %v", requestId, err)
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
		log.Printf("[%s] Error getting zip file info: %v", requestId, err)
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
	log.Printf("[%s] Successfully converted %d files, sending zip (%d bytes)", requestId, len(outputFiles), zipInfo.Size())
	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, zipFile)
	if err != nil {
		log.Printf("[%s] Error streaming zip file: %v", requestId, err)
	}
}

func check_health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func main() {
	http.HandleFunc("/", handler)
	http.HandleFunc("/health", check_health)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
