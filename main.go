// Accepts requirements.txt and optional constraints.txt
// Runs pip install and zips the resulting site-packages directory
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

const workDirPrefix = "pip_work_"

// Accept requirements.txt and optional constraints.txt
// constraints.txt is optional and used for reproducible installs
// The output is a zip of the installed site-packages

type PythonFiles struct {
	RequirementsTXT string `json:"requirements.txt"`
	ConstraintsTXT  string `json:"constraints.txt,omitempty"`
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

	var pyFiles PythonFiles

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle multipart form upload
		err := r.ParseMultipartForm(20 << 20) // 20MB max memory
		if err != nil {
			http.Error(w, "Error parsing multipart form: "+err.Error(), http.StatusBadRequest)
			return
		}
		reqFile, _, err := r.FormFile("requirements.txt")
		if err != nil {
			http.Error(w, "Missing requirements.txt file in form-data", http.StatusBadRequest)
			return
		}
		defer reqFile.Close()
		reqBytes, err := io.ReadAll(reqFile)
		if err != nil {
			http.Error(w, "Error reading requirements.txt: "+err.Error(), http.StatusBadRequest)
			return
		}
		pyFiles.RequirementsTXT = string(reqBytes)

		conFile, _, err := r.FormFile("constraints.txt")
		if err == nil {
			defer conFile.Close()
			conBytes, err := io.ReadAll(conFile)
			if err != nil {
				http.Error(w, "Error reading constraints.txt: "+err.Error(), http.StatusBadRequest)
				return
			}
			pyFiles.ConstraintsTXT = string(conBytes)
		}
	} else {
		// Fallback: JSON body
		err := json.NewDecoder(io.LimitReader(r.Body, 10*1024*1024)).Decode(&pyFiles) // 10MB limit
		if err != nil {
			http.Error(w, fmt.Sprintf("Error decoding request body: %v", err), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()
	}

	if pyFiles.RequirementsTXT == "" {
		http.Error(w, "Missing requirements.txt in request", http.StatusBadRequest)
		return
	}

	// Create a temporary working directory
	tmpDir, err := os.MkdirTemp("", workDirPrefix)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create temp directory: %v", err), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir) // Clean up afterwards

	// Write requirements.txt
	if err := os.WriteFile(filepath.Join(tmpDir, "requirements.txt"), []byte(pyFiles.RequirementsTXT), 0644); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write requirements.txt: %v", err), http.StatusInternalServerError)
		return
	}

	pipArgs := []string{"install", "-r", "requirements.txt", "--target", "site-packages"}
	if pyFiles.ConstraintsTXT != "" {
		if err := os.WriteFile(filepath.Join(tmpDir, "constraints.txt"), []byte(pyFiles.ConstraintsTXT), 0644); err != nil {
			http.Error(w, fmt.Sprintf("Failed to write constraints.txt: %v", err), http.StatusInternalServerError)
			return
		}
		pipArgs = append(pipArgs, "-c", "constraints.txt")
	}

	// Run pip install
	cmd := exec.Command("pip", pipArgs...)
	cmd.Dir = tmpDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Printf("pip install failed in %s. Stderr: %s", tmpDir, stderr.String())
		http.Error(w, fmt.Sprintf("pip install failed: %v\nStderr: %s", err, stderr.String()), http.StatusInternalServerError)
		return
	}
	log.Printf("pip install completed successfully in %s", tmpDir)

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"python_packages.zip\"")

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	// Add site-packages to zip
	sitePackagesPath := filepath.Join(tmpDir, "site-packages")
	err = filepath.Walk(sitePackagesPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(tmpDir, path)
		if err != nil {
			return err
		}
		if relPath == "." || relPath == ".." {
			return nil
		}
		zipPath := filepath.ToSlash(relPath)
		if info.IsDir() {
			if !strings.HasSuffix(zipPath, "/") {
				zipPath += "/"
			}
			_, err = zipWriter.CreateHeader(&zip.FileHeader{
				Name:   zipPath,
				Method: zip.Store,
			})
			if err != nil {
				log.Printf("Failed to create directory header in zip for %s: %v", zipPath, err)
				return err
			}
			return nil
		}
		fileInZip, err := zipWriter.Create(zipPath)
		if err != nil {
			log.Printf("Failed to create zip entry for %s: %v", path, err)
			return err
		}
		fileToZip, err := os.Open(path)
		if err != nil {
			log.Printf("Failed to open file %s for zipping: %v", path, err)
			return err
		}
		defer fileToZip.Close()
		_, err = io.Copy(fileInZip, fileToZip)
		if err != nil {
			log.Printf("Failed to copy file %s to zip: %v", path, err)
			return err
		}
		return nil
	})

	if err != nil {
		log.Printf("Error walking site-packages path %s: %v", sitePackagesPath, err)
		if w.Header().Get("Content-Type") == "" {
			http.Error(w, fmt.Sprintf("Error zipping files: %v", err), http.StatusInternalServerError)
		}
		return
	}
	log.Println("Successfully streamed zip response.")
}

