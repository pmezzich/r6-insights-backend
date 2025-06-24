package main

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"

    "github.com/pmezzich/r6-backend/r6dissect"
)

func uploadHandler(w http.ResponseWriter, r *http.Request) {
    r.ParseMultipartForm(10 << 20) // 10 MB max upload size

    file, handler, err := r.FormFile("file")
    if err != nil {
        http.Error(w, "File upload error", http.StatusBadRequest)
        return
    }
    defer file.Close()

    tempPath := "/tmp/" + handler.Filename
    dst, err := os.Create(tempPath)
    if err != nil {
        http.Error(w, "Failed to save file", http.StatusInternalServerError)
        return
    }
    defer dst.Close()
    io.Copy(dst, file)

    // Call r6-dissect parser
    matchData, err := r6dissect.ParseMatchReplay(tempPath)
    if err != nil {
        http.Error(w, "Error parsing replay: "+err.Error(), http.StatusInternalServerError)
        return
    }

    // Encode result to JSON and send response
    jsonData, err := json.MarshalIndent(matchData, "", "  ")
    if err != nil {
        http.Error(w, "Failed to convert to JSON", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.Write(jsonData)
}

func main() {
    http.HandleFunc("/upload", uploadHandler)
    fmt.Println("Server running on :8080")
    http.ListenAndServe(":8080", nil)
}
