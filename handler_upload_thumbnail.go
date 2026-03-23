package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20 // 10 MB
	r.ParseMultipartForm(maxMemory)
	uploadedFile, header, err := r.FormFile("thumbnail")
	if err != nil {
		fmt.Println("Could not get file and header: ", err)
		respondWithError(w, http.StatusBadRequest, http.StatusText(http.StatusBadRequest), err)
		return
	}
	contentType := header.Header.Get("Content-Type")
	mimeType, _, err := mime.ParseMediaType(contentType)
	if mimeType != "image/jpeg" && mimeType != "image/png" {
		respondWithError(w, http.StatusBadRequest, http.StatusText(http.StatusBadRequest), err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		fmt.Println("Could not find video in database: ", err)
		respondWithError(w, http.StatusBadRequest, http.StatusText(http.StatusBadRequest), err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Video not owned by user", err)
		return
	}
	nonce := make([]byte, 32)
	_, err = rand.Read(nonce)
	if err != nil {
		fmt.Println("Cloud not read rand", err)
		respondWithError(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), err)
		return
	}

	encodedURL := base64.RawURLEncoding.EncodeToString(nonce)
	fileExtension := strings.TrimLeft(contentType, "image/")
	fileName := encodedURL + "." + fileExtension

	filePath := filepath.Join("assets", fileExtension) + fileName
	assetFile, err := os.Create(filePath)
	if err != nil {
		fmt.Println("Cloud not create file", err)
		respondWithError(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), err)
		return
	}

	_, err = io.Copy(assetFile, uploadedFile)
	if err != nil {
		fmt.Println("Cloud not write to file", err)
		respondWithError(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), err)
		return
	}

	thumbnailURL := fmt.Sprintf("http://localhost:%s/%s", cfg.port, filePath)
	video.ThumbnailURL = &thumbnailURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		fmt.Println("Could not update video metadata: ", err)
		respondWithError(w, http.StatusBadRequest, http.StatusText(http.StatusBadRequest), err)
	}

	fmt.Print("Success")
	respondWithJSON(w, http.StatusOK, video)
}
