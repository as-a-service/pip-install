package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const workDirPrefix = "npm_work_"

type PackageFiles struct {
	PackageJSON      string `json:"package.json"`
	PackageLockJSON string `json:"package-lock.json,omitempty"`
}

func main() {
	http.HandleFunc("/install", handleInstall)
	log.Println("Server starting on port 8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func handleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	var packageFiles PackageFiles
	// Limit request body size to prevent abuse
	err := json.NewDecoder(io.LimitReader(r.Body, 10*1024*1024)).Decode(&packageFiles) // 10MB limit
	if err != nil {
		http.Error(w, fmt.Sprintf("Error decoding request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if packageFiles.PackageJSON == "" {
		http.Error(w, "Missing package.json in request body", http.StatusBadRequest)
		return
	}

	// Create a temporary working directory
	tmpDir, err := os.MkdirTemp("", workDirPrefix)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create temp directory: %v", err), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir) // Clean up afterwards

	// Write package.json
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(packageFiles.PackageJSON), 0644); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write package.json: %v", err), http.StatusInternalServerError)
		return
	}

	npmCommand := "install"
	// Write package-lock.json if provided and use 'npm ci'
	if packageFiles.PackageLockJSON != "" {
		if err := os.WriteFile(filepath.Join(tmpDir, "package-lock.json"), []byte(packageFiles.PackageLockJSON), 0644); err != nil {
			http.Error(w, fmt.Sprintf("Failed to write package-lock.json: %v", err), http.StatusInternalServerError)
			return
		}
		npmCommand = "ci"
	}

	// Run npm install or npm ci
	cmd := exec.Command("npm", npmCommand)
	cmd.Dir = tmpDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Printf("npm %s failed in %s. Stderr: %s", npmCommand, tmpDir, stderr.String())
		http.Error(w, fmt.Sprintf("npm %s failed: %v\nStderr: %s", npmCommand, err, stderr.String()), http.StatusInternalServerError)
		return
	}
	log.Printf("npm %s completed successfully in %s", npmCommand, tmpDir)

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"npm_build.zip\"")

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	// Remove package.json and package-lock.json from zip
	filesToZip := []string{}

	// Add package.json and package-lock.json to zip
	for _, file := range filesToZip {
		filePath := filepath.Join(tmpDir, file)
		if _, err := os.Stat(filePath); err == nil {
			f, err := zipWriter.Create(file)
			if err != nil {
				log.Printf("Failed to create zip entry for %s: %v", file, err)
				// Don't send http.Error here as headers might have been written
				return
			}
			content, err := os.ReadFile(filePath)
			if err != nil {
				log.Printf("Failed to read %s for zipping: %v", file, err)
				return
			}
			_, err = f.Write(content)
			if err != nil {
				log.Printf("Failed to write %s to zip: %v", file, err)
				return
			}
		}
	}

	// Add node_modules to zip
	nodeModulesPath := filepath.Join(tmpDir, "node_modules")
	err = filepath.Walk(nodeModulesPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create a proper path for the zip file
		relPath, err := filepath.Rel(tmpDir, path)
		if err != nil {
			return err
		}

		// Skip if it's the root node_modules directory itself
		if relPath == "." || relPath == ".." {
			return nil
		}
		
		// Ensure paths in zip are relative and use forward slashes
		zipPath := filepath.ToSlash(relPath)


		if info.IsDir() {
			// For directories, create a header, but don't write content directly
			// Some zip utilities might require explicit directory entries
			if !strings.HasSuffix(zipPath, "/") {
				zipPath += "/"
			}
			_, err = zipWriter.CreateHeader(&zip.FileHeader{
				Name:   zipPath,
				Method: zip.Store, // Store (no compression) for directories or Deflate
				// Set other metadata if needed, like ModifiedDate
			})
			if err != nil {
				log.Printf("Failed to create directory header in zip for %s: %v", zipPath, err)
				return err
			}
			return nil
		}

		// Create a file entry in the zip
		fileInZip, err := zipWriter.Create(zipPath)
		if err != nil {
			log.Printf("Failed to create zip entry for %s: %v", path, err)
			return err
		}

		// Open the file to be zipped
		fileToZip, err := os.Open(path)
		if err != nil {
			log.Printf("Failed to open file %s for zipping: %v", path, err)
			return err
		}
		defer fileToZip.Close()

		// Copy the file content to the zip entry
		_, err = io.Copy(fileInZip, fileToZip)
		if err != nil {
			log.Printf("Failed to copy file %s to zip: %v", path, err)
			return err
		}
		return nil
	})

	if err != nil {
		// Log error, but response might have already started streaming
		log.Printf("Error walking node_modules path %s: %v", nodeModulesPath, err)
		// Avoid writing http.Error if headers are already sent
		if w.Header().Get("Content-Type") == "" { // A bit of a heuristic
			http.Error(w, fmt.Sprintf("Error zipping files: %v", err), http.StatusInternalServerError)
		}
		return
	}
	log.Println("Successfully streamed zip response.")
}

